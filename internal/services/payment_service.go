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

		var remaining float64
		err := tx.QueryRow(`
			SELECT remaining_amount
			FROM invoices
			WHERE id = $1
			FOR UPDATE
		`, alloc.InvoiceID).Scan(&remaining)

		if err != nil {
			return err
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

func (s *PaymentService) autoAllocateFIFO(
	tx *sql.Tx,
	clientID int64,
	amount float64,
) ([]models.PaymentAllocationDTO, error) {

	rows, err := tx.Query(`
		SELECT id, remaining_amount
		FROM invoices
		WHERE client_id = $1
		  AND remaining_amount > 0
		ORDER BY invoice_date ASC
		FOR UPDATE
	`, clientID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	remaining := amount
	var allocations []models.PaymentAllocationDTO

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
