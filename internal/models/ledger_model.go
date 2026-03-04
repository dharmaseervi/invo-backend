package models

import "time"

type LedgerEntry struct {
	ID          int64     `json:"id"`
	CompanyID   int64     `json:"company_id"`
	ClientID    int64     `json:"client_id"`
	SourceType  string    `json:"source_type"`
	SourceID    int64     `json:"source_id"`
	Debit       float64   `json:"debit"`
	Credit      float64   `json:"credit"`
	Balance     float64   `json:"balance"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
}
