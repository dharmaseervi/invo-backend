package models

type ChallanItemRequest struct {
	ItemID      int    `json:"item_id"`
	Qty         int    `json:"qty"`
	Description string `json:"description"`
}

type ChallanRequestDTO struct {
	CompanyID   int                  `json:"company_id"`
	ClientID    int                  `json:"client_id"`
	ChallanDate string               `json:"challan_date"`
	InvoiceID   *int                 `json:"invoice_id"`
	Notes       string               `json:"notes"`
	Items       []ChallanItemRequest `json:"items"`
}
