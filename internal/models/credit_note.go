package models

import "time"

type CreditNote struct {
	ID         int64   `json:"id"`
	CompanyID  int64   `json:"company_id"`
	ClientID   int64   `json:"client_id"`
	InvoiceID  *int64  `json:"invoice_id"`
	Type       string  `json:"type"` // item | value
	CreditNo   string  `json:"credit_number"`
	CreditDate string  `json:"credit_date"`
	Reason     string  `json:"reason"`
	Subtotal   float64 `json:"subtotal"`
	Tax        float64 `json:"tax"`
	Total      float64 `json:"total"`
}

type CreditNoteRequestDTO struct {
	ClientID   int64               `json:"client_id" binding:"required"`
	InvoiceID  *int64              `json:"invoice_id"`
	Type       string              `json:"type" binding:"required"` // item | value
	CreditDate string              `json:"credit_date" binding:"required"`
	Reason     string              `json:"reason"`
	Items      []CreditNoteItemDTO `json:"items"`
	Amount     float64             `json:"amount"`
}

type CreditNoteItemDTO struct {
	ItemID  int64   `json:"item_id"`
	Qty     float64 `json:"qty"`
	Rate    float64 `json:"rate"`
	TaxRate float64 `json:"tax_rate"`
}

type CreditNoteListDTO struct {
	ID           int64     `json:"id"`
	CreditNumber string    `json:"credit_number"`
	ClientID     int64     `json:"client_id"`
	ClientName   string    `json:"client_name"`
	Type         string    `json:"type"`
	Total        float64   `json:"total"`
	Balance      float64   `json:"balance"`
	Status       string    `json:"status"`
	CreditDate   time.Time `json:"credit_date"`
}

type CreditNoteDetailDTO struct {
	ID           int64     `json:"id"`
	CreditNumber string    `json:"credit_number"`
	ClientID     int64     `json:"client_id"`
	ClientName   string    `json:"client_name"`
	InvoiceID    *int64    `json:"invoice_id,omitempty"`
	Type         string    `json:"type"`
	Reason       *string   `json:"reason,omitempty"`
	Subtotal     float64   `json:"subtotal"`
	Tax          float64   `json:"tax"`
	Total        float64   `json:"total"`
	Balance      float64   `json:"balance"`
	Status       string    `json:"status"`
	CreditDate   time.Time `json:"credit_date"`
	CreatedAt    time.Time `json:"created_at"`
}

type CreditNoteItemResponse struct {
	ID       int64   `json:"id"`
	ItemID   int64   `json:"item_id"`
	ItemName string  `json:"item_name"`
	Qty      float64 `json:"qty"`
	Rate     float64 `json:"rate"`
	TaxRate  float64 `json:"tax_rate"`
	Total    float64 `json:"total"`
}

type CreditNoteDetailResponse struct {
	ID           int64  `json:"id"`
	CreditNumber string `json:"credit_number"`

	ClientID   int64  `json:"client_id"`
	ClientName string `json:"client_name"`

	InvoiceID     *int64  `json:"invoice_id"`
	InvoiceNumber *string `json:"invoice_number"`

	Type   string  `json:"type"`
	Reason *string `json:"reason"`

	Subtotal float64 `json:"subtotal"`
	Tax      float64 `json:"tax"`
	Total    float64 `json:"total"`
	Balance  float64 `json:"balance"`

	Status     string `json:"status"`
	CreditDate string `json:"credit_date"`
	CreatedAt  string `json:"created_at"`

	Items []CreditNoteItemResponse `json:"items"`
}
