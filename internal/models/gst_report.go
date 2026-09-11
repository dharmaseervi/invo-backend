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

// GSTCreditNoteRow is one line of the CDNR section — credit and debit notes issued
// against registered customers. Without it a return reduces what the customer owes
// while the return still declares the original supply in full.
type GSTCreditNoteRow struct {
	CreditNoteID    int     `json:"credit_note_id"`
	CreditNumber    string  `json:"credit_number"`
	CreditDate      string  `json:"credit_date"`
	ClientName      string  `json:"client_name"`
	ClientGSTIN     string  `json:"client_gstin"`
	OriginalInvoice string  `json:"original_invoice"`
	Reason          string  `json:"reason"`
	TaxableValue    float64 `json:"taxable_value"`
	CGST            float64 `json:"cgst"`
	SGST            float64 `json:"sgst"`
	IGST            float64 `json:"igst"`
	Total           float64 `json:"total"`
}

type GSTReportResponse struct {
	Start        string                 `json:"start"`
	End          string                 `json:"end"`
	CompanyState string                 `json:"company_state"`
	Summary      GSTSummary             `json:"summary"`
	Invoices     []GSTInvoiceSummaryRow `json:"invoices"`
	HSNSummary   []GSTHSNSummaryRow     `json:"hsn_summary"`
	// CreditNotes is the CDNR section. NetSummary is Summary less these, which is the
	// figure actually payable.
	CreditNotes []GSTCreditNoteRow `json:"credit_notes"`
	NetSummary  GSTSummary         `json:"net_summary"`
}
