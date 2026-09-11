package models

import "time"

type Payment struct {
	ID            int64     `json:"id"`
	CompanyID     int64     `json:"company_id"`
	ClientID      int64     `json:"client_id"`
	InvoiceID     int64     `json:"invoice_id"`
	Amount        float64   `json:"amount"`
	PaymentMethod string    `json:"payment_method"`
	Reference     string    `json:"reference"`
	Notes         string    `json:"notes"`
	CreatedAt     time.Time `json:"created_at"`
}
type PaymentRequestDTO struct {
	ClientID      int64   `json:"client_id" binding:"required"`
	Amount        float64 `json:"amount" binding:"required,gt=0"`
	PaymentMethod string  `json:"payment_method" binding:"required"`
	Reference     string  `json:"reference"`
	Notes         string  `json:"notes"`

	// PaymentDate is when the money actually changed hands, which is not always when
	// it was entered — yesterday's cheque keyed in this morning belongs to yesterday
	// for every report that groups by date. Defaults to today when omitted.
	PaymentDate *string `json:"payment_date"` // YYYY-MM-DD

	// OPTIONAL: manual allocation (advanced users only)
	Allocations []PaymentAllocationDTO `json:"allocations,omitempty"`
}

// PaymentHistoryRow is one entry in a company's payment history, with the invoices the
// payment was applied to summarised inline.
type PaymentHistoryRow struct {
	ID            int64   `json:"id"`
	ClientID      int64   `json:"client_id"`
	ClientName    string  `json:"client_name"`
	Amount        float64 `json:"amount"`
	PaymentMethod string  `json:"payment_method"`
	Reference     string  `json:"reference"`
	Notes         string  `json:"notes"`
	PaymentDate   string  `json:"payment_date"`
	CreatedAt     string  `json:"created_at"`
	AppliedTo     string  `json:"applied_to"`
}

type PaymentAllocationDTO struct {
	InvoiceID int64   `json:"invoice_id" binding:"required"`
	Amount    float64 `json:"amount" binding:"required,gt=0"`
}
