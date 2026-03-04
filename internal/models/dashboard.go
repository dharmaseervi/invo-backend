// internal/models/dashboard.go

package models

type DashboardResponse struct {
	Period  string          `json:"period"`
	Revenue RevenueBlock    `json:"revenue"`
	Counts  CountBlock      `json:"counts"`
	Recent  []RecentInvoice `json:"recent_invoices"`
}

type RevenueBlock struct {
	Total         float64 `json:"total"`
	ChangePercent float64 `json:"change_percent"`
}

type CountBlock struct {
	Invoices int `json:"invoices"`
	Clients  int `json:"clients"`
	Items    int `json:"items"`
}

type RecentInvoice struct {
	ID         int     `json:"id"`
	InvoiceNo  string  `json:"invoice_number"`
	ClientName string  `json:"client_name"`
	Total      float64 `json:"total"`
	Status     string  `json:"status"`
	CreatedAt  string  `json:"created_at"`
}
