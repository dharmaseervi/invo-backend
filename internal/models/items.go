package models

import "time"

type Item struct {
	ID          int    `json:"id"`
	UserID      int    `json:"user_id"`
	CompanyID   int    `json:"company_id"`
	Name        string `json:"name"`
	Category    int    `json:"category"`
	HSNCode     string `json:"hsn_code"`
	SKU         string `json:"sku"`
	Unit        string `json:"unit"`
	Description string `json:"description"`
	// Nullable: an item need not belong to a category. Scanning NULL into a plain int
	// fails, and the list handler used to swallow that error and drop the row, so
	// uncategorised items silently disappeared from the catalogue.
	CategoryID    *int      `json:"category_id"`
	CostPrice     float64   `json:"cost_price"`
	Price         float64   `json:"price"`
	Quantity      int       `json:"quantity"`
	LowStockAlert int       `json:"low_stock_alert"`
	TaxRate       float64   `json:"tax_rate"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}
