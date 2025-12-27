package models

type InvoiceItemRequest struct {
	ItemID   int     `json:"item_id"`
	Qty      int     `json:"qty"`
	Rate     float64 `json:"rate"`
	Discount float64 `json:"discount"`
	TaxRate  float64 `json:"tax_rate"`
}

type InvoiceRequestDTO struct {
	CompanyID   int                  `json:"company_id"`
	ClientID    int                  `json:"client_id"`
	InvoiceDate string               `json:"invoice_date"` // YYYY-MM-DD
	DueDate     string               `json:"due_date"`
	Notes       *string              `json:"notes"`
	Items       []InvoiceItemRequest `json:"items"`
}

type Invoice struct {
	ID              int     `json:"id"`
	InvoiceNumber   string  `json:"invoice_number"`
	ClientID        int     `json:"client_id"`
	InvoiceDate     string  `json:"invoice_date"`
	DueDate         string  `json:"due_date"`
	Status          string  `json:"status"`
	Subtotal        float64 `json:"subtotal"`
	Tax             float64 `json:"tax"`
	Total           float64 `json:"total"`
	PaidAmount      float64 `json:"paid_amount"`
	RemainingAmount float64 `json:"remaining_amount"`
	CreatedAt       string  `json:"created_at"`
}
