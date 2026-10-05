package services

import (
	"database/sql"
	"strings"

	"invo-server/internal/models"
	"invo-server/internal/money"
)

// Putting a payment right, and giving money back.
//
// Nothing here deletes anything. A payment entered against the wrong customer is still
// a thing that happened: the row stays, marked reversed, with an entry in the ledger
// that undoes it. A customer disputing their balance is shown a history, and a history
// with rows quietly removed from it cannot be explained — "it was there last month" is
// the end of that conversation.

// ReversePayment undoes a payment: the invoices it settled go back to owing, the
// customer's balance goes back up, and the payment is marked reversed with the reason.
func (s *PaymentService) ReversePayment(
	companyID, paymentID int64,
	reason string,
) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Locked, because two taps on Reverse would otherwise both read "recorded" and
	// each put the money back — an invoice would owe twice what it does.
	var clientID int64
	var amount float64
	var status string
	err = tx.QueryRow(`
		SELECT client_id, amount, status
		FROM payments
		WHERE id = $1 AND company_id = $2
		FOR UPDATE
	`, paymentID, companyID).Scan(&clientID, &amount, &status)

	if err == sql.ErrNoRows {
		return PaymentInputError{"That payment isn't one of this company's."}
	}
	if err != nil {
		return err
	}
	if status == "reversed" {
		return PaymentInputError{"That payment has already been reversed."}
	}

	if err := unapplyAllocations(tx, paymentID); err != nil {
		return err
	}

	if _, err := tx.Exec(`
		UPDATE payments
		SET status = 'reversed', reversed_at = NOW(), reversal_reason = $2, unapplied_amount = 0
		WHERE id = $1
	`, paymentID, strings.TrimSpace(reason)); err != nil {
		return err
	}

	// A debit of the same amount: the credit the payment wrote is cancelled, and both
	// lines stay on the statement so the correction is visible rather than silent.
	narration := "Payment reversed"
	if r := strings.TrimSpace(reason); r != "" {
		narration += " — " + r
	}
	if err := s.ledger.AddEntryTx(
		tx, companyID, clientID, "PAYMENT_REVERSAL", paymentID, amount, 0, narration,
	); err != nil {
		return err
	}

	return tx.Commit()
}

// ReallocatePayment moves a payment onto different invoices, for the common case of the
// right money against the wrong bill.
//
// The payment itself is untouched — same amount, same date, same receipt number. Only
// what it settles changes, which is what actually went wrong.
func (s *PaymentService) ReallocatePayment(
	companyID, paymentID int64,
	allocations []models.PaymentAllocationDTO,
) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var clientID int64
	var amount float64
	var status string
	err = tx.QueryRow(`
		SELECT client_id, amount, status
		FROM payments
		WHERE id = $1 AND company_id = $2
		FOR UPDATE
	`, paymentID, companyID).Scan(&clientID, &amount, &status)

	if err == sql.ErrNoRows {
		return PaymentInputError{"That payment isn't one of this company's."}
	}
	if err != nil {
		return err
	}
	if status == "reversed" {
		return PaymentInputError{"That payment was reversed, so there is nothing to move."}
	}

	total := money.Zero()
	for _, a := range allocations {
		if a.Amount <= 0 {
			return PaymentInputError{"Each invoice needs an amount greater than zero."}
		}
		total = total.Add(money.FromFloat(a.Amount))
	}
	if total.GreaterThan(money.FromFloat(amount)) {
		return PaymentInputError{"That applies more to invoices than the payment is for."}
	}

	// Take it all back first, then apply afresh. Working out the difference per invoice
	// would be the same thing with more ways to be wrong, and this runs in one
	// transaction so the books are never between the two states.
	if err := unapplyAllocations(tx, paymentID); err != nil {
		return err
	}

	for _, a := range allocations {
		var remaining float64
		var invoiceStatus string
		err := tx.QueryRow(`
			SELECT remaining_amount, status
			FROM invoices
			WHERE id = $1 AND company_id = $2 AND client_id = $3
			FOR UPDATE
		`, a.InvoiceID, companyID, clientID).Scan(&remaining, &invoiceStatus)

		if err == sql.ErrNoRows {
			// Scoped to the client as well as the company: a payment cannot be moved
			// onto another customer's invoice.
			return PaymentInputError{"One of those invoices isn't this client's."}
		}
		if err != nil {
			return err
		}
		if !isPayableStatus(invoiceStatus) && invoiceStatus != "paid" {
			return PaymentInputError{"Payments can only go against an issued or part-paid invoice."}
		}
		if money.FromFloat(a.Amount).GreaterThan(money.FromFloat(remaining)) {
			return PaymentInputError{"That's more than the invoice still owes."}
		}

		if _, err := tx.Exec(`
			INSERT INTO payment_allocations (payment_id, invoice_id, amount)
			VALUES ($1,$2,$3)
		`, paymentID, a.InvoiceID, a.Amount); err != nil {
			return err
		}

		if _, err := tx.Exec(`
			UPDATE invoices
			SET paid_amount = paid_amount + $1,
			    remaining_amount = remaining_amount - $1,
			    status = CASE WHEN remaining_amount - $1 <= 0 THEN 'paid' ELSE 'partial' END,
			    updated_at = NOW()
			WHERE id = $2
		`, a.Amount, a.InvoiceID); err != nil {
			return err
		}
	}

	unapplied := money.FromFloat(amount).Sub(total).Round()
	if _, err := tx.Exec(
		`UPDATE payments SET unapplied_amount = $2, updated_at = NOW() WHERE id = $1`,
		paymentID, unapplied.Float64(),
	); err != nil {
		return err
	}

	// No ledger entry: the customer's balance has not moved. The same money is simply
	// against different invoices, and writing a pair of entries that cancel out would
	// only make a statement harder to read.
	return tx.Commit()
}

// unapplyAllocations takes a payment back off the invoices it settled and deletes the
// allocation rows.
//
// The status is recomputed from what is left owing rather than set to a fixed value: an
// invoice that was paid by two payments is still partial after one is taken off, and
// one that had nothing else against it goes back to issued.
func unapplyAllocations(tx *sql.Tx, paymentID int64) error {
	rows, err := tx.Query(
		`SELECT invoice_id, amount FROM payment_allocations WHERE payment_id = $1`, paymentID)
	if err != nil {
		return err
	}
	type applied struct {
		invoiceID int64
		amount    float64
	}
	var list []applied
	for rows.Next() {
		var a applied
		if err := rows.Scan(&a.invoiceID, &a.amount); err != nil {
			rows.Close()
			return err
		}
		list = append(list, a)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	for _, a := range list {
		if _, err := tx.Exec(`
			UPDATE invoices
			SET paid_amount = GREATEST(paid_amount - $1, 0),
			    remaining_amount = remaining_amount + $1,
			    status = CASE
			        WHEN GREATEST(paid_amount - $1, 0) <= 0 THEN 'issued'
			        ELSE 'partial'
			    END,
			    updated_at = NOW()
			WHERE id = $2 AND status <> 'cancelled'
		`, a.amount, a.invoiceID); err != nil {
			return err
		}
	}

	_, err = tx.Exec(`DELETE FROM payment_allocations WHERE payment_id = $1`, paymentID)
	return err
}

// RefundRequest is money going back to a customer.
type RefundRequest struct {
	ClientID     int64   `json:"client_id"`
	CreditNoteID *int64  `json:"credit_note_id"`
	Amount       float64 `json:"amount"`
	Method       string  `json:"method"`
	Reference    string  `json:"reference"`
	Notes        string  `json:"notes"`
	RefundDate   *string `json:"refund_date"`
}

// RecordRefund hands money back: against a credit note, or returning an advance the
// customer is not going to use.
//
// A credit note on its own does not move any money — it says the customer is owed
// something. This is the part where they actually get it, which is why it is a separate
// record rather than a flag on the credit note.
func (s *PaymentService) RecordRefund(
	companyID int64,
	req RefundRequest,
) (int64, error) {
	if req.Amount <= 0 {
		return 0, PaymentInputError{"Enter how much is being refunded."}
	}
	if strings.TrimSpace(req.Method) == "" {
		return 0, PaymentInputError{"Say how the money was returned — cash, UPI, bank transfer."}
	}

	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	var clientOK bool
	if err := tx.QueryRow(
		`SELECT EXISTS(SELECT 1 FROM clients WHERE id = $1 AND company_id = $2)`,
		req.ClientID, companyID,
	).Scan(&clientOK); err != nil {
		return 0, err
	}
	if !clientOK {
		return 0, PaymentInputError{"That customer isn't one of this company's."}
	}

	if req.CreditNoteID != nil {
		// Locked while its balance is checked and reduced, so two refunds against one
		// credit note cannot both pass.
		var balance float64
		var status string
		err := tx.QueryRow(`
			SELECT balance, status FROM credit_notes
			WHERE id = $1 AND company_id = $2 AND client_id = $3
			FOR UPDATE
		`, *req.CreditNoteID, companyID, req.ClientID).Scan(&balance, &status)

		if err == sql.ErrNoRows {
			return 0, PaymentInputError{"That credit note isn't this customer's."}
		}
		if err != nil {
			return 0, err
		}
		if status == "cancelled" {
			return 0, PaymentInputError{"That credit note has been cancelled."}
		}
		if money.FromFloat(req.Amount).GreaterThan(money.FromFloat(balance)) {
			return 0, PaymentInputError{
				"That's more than the credit note has left — " + money.FromFloat(balance).String() + ".",
			}
		}

		if _, err := tx.Exec(`
			UPDATE credit_notes
			SET balance = balance - $2,
			    status = CASE WHEN balance - $2 <= 0 THEN 'settled' ELSE status END
			WHERE id = $1
		`, *req.CreditNoteID, req.Amount); err != nil {
			return 0, err
		}
	}

	var refundID int64
	if err := tx.QueryRow(`
		INSERT INTO refunds
			(company_id, client_id, credit_note_id, amount, method, reference, notes, refund_date)
		VALUES ($1,$2,$3,$4,$5,$6,$7, COALESCE($8::date, CURRENT_DATE))
		RETURNING id
	`,
		companyID, req.ClientID, req.CreditNoteID, req.Amount,
		strings.TrimSpace(req.Method), req.Reference, req.Notes, req.RefundDate,
	).Scan(&refundID); err != nil {
		return 0, err
	}

	// A debit: money has gone back, so whatever the customer was holding in credit is
	// that much smaller — and if they owed nothing, they now owe this.
	narration := "Refund paid"
	if r := strings.TrimSpace(req.Reference); r != "" {
		narration += " — " + r
	}
	if err := s.ledger.AddEntryTx(
		tx, companyID, req.ClientID, "REFUND", refundID, req.Amount, 0, narration,
	); err != nil {
		return 0, err
	}

	return refundID, tx.Commit()
}
