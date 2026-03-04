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

	// OPTIONAL: manual allocation (advanced users only)
	Allocations []PaymentAllocationDTO `json:"allocations,omitempty"`
}

type PaymentAllocationDTO struct {
	InvoiceID int64   `json:"invoice_id" binding:"required"`
	Amount    float64 `json:"amount" binding:"required,gt=0"`
}
