package services

import (
	"database/sql"
	"errors"
	"invo-server/internal/models"
	"math"
)

type PaymentService struct {
	db     *sql.DB
	ledger *LedgerService
}

func NewPaymentService(db *sql.DB, ledger *LedgerService) *PaymentService {
	return &PaymentService{db: db, ledger: ledger}
}

func (s *PaymentService) RecordPaymentTx(
	tx *sql.Tx,
	companyID int64,
	clientID int64,
	req models.PaymentRequestDTO,
) error {

	// 1️⃣ Auto-allocate if allocations not provided
	if len(req.Allocations) == 0 {
		allocations, err := s.autoAllocateFIFO(
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

	// 2️⃣ Validate allocation total
	var allocated float64
	for _, a := range req.Allocations {
		allocated += a.Amount
	}

	// float-safe comparison
	if math.Abs(allocated-req.Amount) > 0.01 {
		return errors.New("allocation total does not match payment amount")
	}

	// 3️⃣ Insert payment
	var paymentID int64
	err := tx.QueryRow(`
		INSERT INTO payments (
			company_id,
			client_id,
			amount,
			payment_method,
			reference,
			notes
		)
		VALUES ($1,$2,$3,$4,$5,$6)
		RETURNING id
	`,
		companyID,
		clientID,
		req.Amount,
		req.PaymentMethod,
		req.Reference,
		req.Notes,
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
		err := tx.QueryRow(`
			SELECT remaining_amount, status
			FROM invoices
			WHERE id = $1 AND company_id = $2 AND client_id = $3
			FOR UPDATE
		`, alloc.InvoiceID, companyID, clientID).Scan(&remaining, &status)

		if err == sql.ErrNoRows {
			return errors.New("invoice does not belong to this client")
		}
		if err != nil {
			return err
		}

		if !isPayableStatus(status) {
			return errors.New("invoice is not open for payment")
		}

		if alloc.Amount > remaining {
			return errors.New("allocation exceeds invoice balance")
		}

		// save allocation
		_, err = tx.Exec(`
			INSERT INTO payment_allocations
				(payment_id, invoice_id, amount)
			VALUES ($1,$2,$3)
		`, paymentID, alloc.InvoiceID, alloc.Amount)

		if err != nil {
			return err
		}

		// update invoice
		_, err = tx.Exec(`
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
	return s.ledger.AddEntryTx(
		tx,
		companyID,
		clientID,
		"PAYMENT",
		paymentID,
		0,
		req.Amount,
		"Payment received",
	)
}

// isPayableStatus reports whether an invoice can take a payment. A draft has not been
// issued yet (its number and stock movement are still pending) and a cancelled invoice
// is closed — settling either one leaves it in a state it can never be issued from.
func isPayableStatus(status string) bool {
	return status == "issued" || status == "partial"
}

func (s *PaymentService) autoAllocateFIFO(
	tx *sql.Tx,
	companyID int64,
	clientID int64,
	amount float64,
) ([]models.PaymentAllocationDTO, error) {

	rows, err := tx.Query(`
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

	remaining := amount
	allocations := []models.PaymentAllocationDTO{}

	for rows.Next() && remaining > 0 {
		var invoiceID int64
		var due float64

		if err := rows.Scan(&invoiceID, &due); err != nil {
			return nil, err
		}

		applied := due
		if remaining < due {
			applied = remaining
		}

		allocations = append(allocations, models.PaymentAllocationDTO{
			InvoiceID: invoiceID,
			Amount:    applied,
		})

		remaining -= applied
	}

	if remaining > 0 {
		return nil, errors.New("payment exceeds outstanding balance")
	}

	return allocations, nil
}
