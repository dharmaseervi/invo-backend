package models

import "time"

type LedgerEntry struct {
	ID          int64     `json:"id"`
	CompanyID   int64     `json:"company_id"`
	ClientID    int64     `json:"client_id"`
	ClientName  string    `json:"client_name"`
	SourceType  string    `json:"source_type"`
	SourceID    int64     `json:"source_id"`
	Debit       float64   `json:"debit"`
	Credit      float64   `json:"credit"`
	Balance     float64   `json:"balance"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
}

// LedgerSummary is a customer's standing: what they have been invoiced, what they have
// paid or been credited, and where that leaves them.
//
// Worked out by the database over every entry, because the screens that show these
// figures only hold a page of rows. Summing a page and labelling it the customer's
// total is how a statement comes to disagree with itself.
type LedgerSummary struct {
	ClientID    int64      `json:"client_id"`
	ClientName  string     `json:"client_name"`
	Debit       float64    `json:"debit"`
	Credit      float64    `json:"credit"`
	Balance     float64    `json:"balance"`
	Entries     int        `json:"entries"`
	LastEntryAt *time.Time `json:"last_entry_at"`
}
