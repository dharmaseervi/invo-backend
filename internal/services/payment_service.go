package services

import (
	"context"
	"database/sql"
	"invo-server/internal/models"
	"invo-server/internal/money"
	"math"
	"sort"
	"strings"
	"time"
)

type PaymentService struct {
	db     *sql.DB
	ledger *LedgerService
}

func NewPaymentService(db *sql.DB, ledger *LedgerService) *PaymentService {
	return &PaymentService{db: db, ledger: ledger}
}

func (s *PaymentService) RecordPaymentTx(ctx context.Context,
	tx *sql.Tx,
	companyID int64,
	clientID int64,
	req models.PaymentRequestDTO,
) error {

	if math.IsNaN(req.Amount) || math.IsInf(req.Amount, 0) || math.IsNaN(req.TDSAmount) || math.IsInf(req.TDSAmount, 0) || req.Amount < 0 || req.TDSAmount < 0 {
		return PaymentInputError{"Enter valid received and withholding amounts."}
	}
	received, withheld := money.FromFloat(req.Amount), money.FromFloat(req.TDSAmount)
	if !received.Add(withheld).GreaterThan(money.Zero()) {
		return PaymentInputError{"Enter money received or tax actually withheld."}
	}
	switch req.PaymentMethod {
	case "cash", "upi", "bank_transfer", "cheque":
	default:
		return PaymentInputError{"Choose a valid payment method."}
	}
	if req.PaymentDate != nil {
		if _, err := time.Parse(time.DateOnly, *req.PaymentDate); err != nil {
			return PaymentInputError{"Enter a valid payment date."}
		}
	}
	if withheld.GreaterThan(money.Zero()) && len(req.Allocations) == 0 {
		return PaymentInputError{"Allocate withholding to a specific invoice."}
	}
	req.Reference = strings.TrimSpace(req.Reference)
	// Lock invoices in one consistent order for mixed-invoice settlements.
	sort.Slice(req.Allocations, func(i, j int) bool { return req.Allocations[i].InvoiceID < req.Allocations[j].InvoiceID })

	// 1️⃣ Auto-allocate if allocations not provided
	if len(req.Allocations) == 0 {
		allocations, err := s.autoAllocateFIFO(ctx,
			tx,
			companyID,
			clientID,
			req.Amount,
		)
		if err != nil {
			return err
		}
		req.Allocations = allocations
	}

	// 2️⃣ Validate allocation total.
	//
	// Exact equality, in decimal. The old check tolerated a paisa either way because
	// float64 sums drift — but that tolerance silently accepted a genuine one-paisa
	// mismatch, leaving the payment and its allocations permanently out of balance.
	allocated, allocatedTDS := money.Zero(), money.Zero()
	seen := map[int64]bool{}
	for _, a := range req.Allocations {
		if a.InvoiceID <= 0 || seen[a.InvoiceID] || math.IsNaN(a.Amount) || math.IsInf(a.Amount, 0) || a.Amount <= 0 || math.IsNaN(a.TDSAmount) || math.IsInf(a.TDSAmount, 0) || a.TDSAmount < 0 || money.FromFloat(a.TDSAmount).GreaterThan(money.FromFloat(a.Amount)) {
			return PaymentInputError{"Each invoice needs one valid settlement with withholding no greater than its amount."}
		}
		seen[a.InvoiceID] = true
		allocated = allocated.Add(money.FromFloat(a.Amount))
		allocatedTDS = allocatedTDS.Add(money.FromFloat(a.TDSAmount))
	}
	if !allocatedTDS.Equal(withheld) {
		return PaymentInputError{"Allocated withholding must equal the tax actually withheld."}
	}

	// Allocations may be less than the payment — the difference is the advance. More
	// than the payment is always a mistake.
	if allocated.GreaterThan(received.Add(withheld)) {
		return PaymentInputError{"That applies more to invoices than the payment is for."}
	}
	unapplied := received.Add(withheld).Sub(allocated).Round()

	// 3️⃣ Insert payment
	var paymentID int64
	err := tx.QueryRowContext(ctx,
		`
		INSERT INTO payments (
			company_id,
			client_id,
			amount,
			payment_method,
			reference,
			notes,
			payment_date,
			unapplied_amount, tds_amount
		)
		VALUES ($1,$2,$3,$4,$5,$6, COALESCE($7::date, CURRENT_DATE), $8, $9)
		RETURNING id
	`,
		companyID,
		clientID,
		req.Amount,
		req.PaymentMethod,
		req.Reference,
		req.Notes,
		req.PaymentDate,
		unapplied.Float64(), withheld.Float64(),
	).Scan(&paymentID)

	if err != nil {
		return err
	}

	// 4️⃣ Apply allocations
	for _, alloc := range req.Allocations {

		// Scoping this lookup to the caller's own company AND client is what stops a
		// crafted allocation from settling an invoice that belongs to someone else —
		// the handler only ever verifies the client, never the invoice ids.
		var remaining float64
		var status string
		err := tx.QueryRowContext(ctx,
			`
			SELECT remaining_amount, status
			FROM invoices
			WHERE id = $1 AND company_id = $2 AND client_id = $3
			FOR UPDATE
		`, alloc.InvoiceID, companyID, clientID).Scan(&remaining, &status)

		if err == sql.ErrNoRows {
			return PaymentInputError{"One of those invoices isn't this client's."}
		}
		if err != nil {
			return err
		}

		if !isPayableStatus(status) {
			return PaymentInputError{"Payments can only go against an issued or part-paid invoice."}
		}

		// Decimal comparison: in float64 an allocation that exactly settles an invoice
		// can compare as greater than the balance and be rejected outright.
		if money.FromFloat(alloc.Amount).GreaterThan(money.FromFloat(remaining)) {
			return PaymentInputError{"That's more than the invoice still owes."}
		}

		// save allocation
		_, err = tx.ExecContext(ctx,
			`
			INSERT INTO payment_allocations
				(payment_id, invoice_id, amount, tds_amount)
			VALUES ($1,$2,$3,$4)
		`, paymentID, alloc.InvoiceID, alloc.Amount, alloc.TDSAmount)

		if err != nil {
			return err
		}

		// update invoice
		_, err = tx.ExecContext(ctx,
			`
			UPDATE invoices
			SET
				paid_amount = paid_amount + $1,
				remaining_amount = remaining_amount - $1,
				status = CASE
					WHEN remaining_amount - $1 <= 0 THEN 'paid'
					ELSE 'partial'
				END
			WHERE id = $2
		`, alloc.Amount, alloc.InvoiceID)

		if err != nil {
			return err
		}
	}

	// 5️⃣ Ledger entry (ONE credit entry)
	//
	// The whole payment is credited, including any advance: the customer has handed
	// over that money whether or not an invoice exists to put it against, and their
	// balance should say so the moment they do.
	narration := "Payment received"
	if unapplied.GreaterThan(money.Zero()) {
		narration = "Payment received (" + unapplied.String() + " on account)"
	}

	if received.GreaterThan(money.Zero()) {
		if err := s.ledger.AddEntryTx(ctx, tx, companyID, clientID, "PAYMENT", paymentID, 0, received.Float64(), narration); err != nil {
			return err
		}
	}
	if withheld.GreaterThan(money.Zero()) {
		return s.ledger.AddEntryTx(ctx, tx, companyID, clientID, "ADJUSTMENT", paymentID, 0, withheld.Float64(), "TDS withheld — tax settlement, not money received")
	}
	return nil
}

// isPayableStatus reports whether an invoice can take a payment. A draft has not been
// issued yet (its number and stock movement are still pending) and a cancelled invoice
// is closed — settling either one leaves it in a state it can never be issued from.
func isPayableStatus(status string) bool {
	return status == "issued" || status == "partial"
}

func (s *PaymentService) autoAllocateFIFO(ctx context.Context,
	tx *sql.Tx,
	companyID int64,
	clientID int64,
	amount float64,
) ([]models.PaymentAllocationDTO, error) {

	rows, err := tx.QueryContext(ctx,
		`
		SELECT id, remaining_amount
		FROM invoices
		WHERE client_id = $1
		  AND company_id = $2
		  AND status IN ('issued', 'partial')
		  AND remaining_amount > 0
		ORDER BY invoice_date ASC
		FOR UPDATE
	`, clientID, companyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// Tracked in decimal. Accumulating this in float64 leaves a residue like 1e-13
	// after the last invoice is settled, and the check below then reports a payment
	// that exactly clears the balance as exceeding it.
	remaining := money.FromFloat(amount)
	allocations := []models.PaymentAllocationDTO{}

	for rows.Next() && remaining.GreaterThan(money.Zero()) {
		var invoiceID int64
		var dueFloat float64

		if err := rows.Scan(&invoiceID, &dueFloat); err != nil {
			return nil, err
		}

		due := money.FromFloat(dueFloat)
		applied := money.Min(remaining, due)

		allocations = append(allocations, models.PaymentAllocationDTO{
			InvoiceID: invoiceID,
			Amount:    applied.Float64(),
		})

		remaining = remaining.Sub(applied)
	}

	// Anything left over is an advance, not a mistake.
	//
	// This used to refuse the payment outright, which made a deposit impossible to
	// record: a customer paying ₹10,000 up front against nothing, or ₹500 more than
	// their invoices because that is the note they had, simply could not be entered.
	// The remainder stays unallocated and sits on their ledger as credit, which the
	// next invoice draws on.
	return allocations, nil
}

// PaymentInputError is a refusal the person can act on, shown to them as is. Before,
// every refusal reached the app as "Failed to record payment", which says nothing about
// what to change.
type PaymentInputError struct{ Msg string }

func (e PaymentInputError) Error() string { return e.Msg }
