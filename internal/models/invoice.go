package models

type InvoiceItemRequest struct {
	ItemID   int     `json:"item_id"`
	Qty      int     `json:"qty"`
	Rate     float64 `json:"rate"`
	Discount float64 `json:"discount"`
	TaxRate  float64 `json:"tax_rate"`
}

type InvoiceRequestDTO struct {
	CompanyID     int                  `json:"company_id"`
	ClientID      int                  `json:"client_id"`
	InvoiceNumber string               `json:"invoice_number"`
	InvoiceDate   string               `json:"invoice_date"` // "yyyy-MM-dd"
	DueDate       string               `json:"due_date"`
	Subtotal      float64              `json:"subtotal"`
	Tax           float64              `json:"tax"`
	Total         float64              `json:"total"`
	Items         []InvoiceItemRequest `json:"items"`
}
