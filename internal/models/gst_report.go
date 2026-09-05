package models

type GSTHSNSummaryRow struct {
	HSNCode      string  `json:"hsn_code"`
	TaxRate      float64 `json:"tax_rate"`
	TotalQty     float64 `json:"total_qty"`
	TaxableValue float64 `json:"taxable_value"`
	CGST         float64 `json:"cgst"`
	SGST         float64 `json:"sgst"`
	IGST         float64 `json:"igst"`
	TotalValue   float64 `json:"total_value"`
}

type GSTInvoiceSummaryRow struct {
	InvoiceID     int     `json:"invoice_id"`
	InvoiceNumber string  `json:"invoice_number"`
	InvoiceDate   string  `json:"invoice_date"`
	ClientName    string  `json:"client_name"`
	ClientGSTIN   string  `json:"client_gstin"`
	PlaceOfSupply string  `json:"place_of_supply"`
	TaxableValue  float64 `json:"taxable_value"`
	CGST          float64 `json:"cgst"`
	SGST          float64 `json:"sgst"`
	IGST          float64 `json:"igst"`
	Total         float64 `json:"total"`
}

type GSTSummary struct {
	InvoiceCount int     `json:"invoice_count"`
	TaxableValue float64 `json:"taxable_value"`
	CGST         float64 `json:"cgst"`
	SGST         float64 `json:"sgst"`
	IGST         float64 `json:"igst"`
	Total        float64 `json:"total"`
}

type GSTReportResponse struct {
	Start        string                 `json:"start"`
	End          string                 `json:"end"`
	CompanyState string                 `json:"company_state"`
	Summary      GSTSummary             `json:"summary"`
	Invoices     []GSTInvoiceSummaryRow `json:"invoices"`
	HSNSummary   []GSTHSNSummaryRow     `json:"hsn_summary"`
}
