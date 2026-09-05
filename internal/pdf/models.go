package pdf

// HeaderTitle returns the printed document title — "TAX INVOICE"/"Invoice" by
// default, or the explicit DocType (e.g. "QUOTATION") when set.
func (i Invoice) HeaderTitle(defaultTitle string) string {
	if i.DocType != "" {
		return i.DocType
	}
	return defaultTitle
}

type InvoicePDFData struct {
	Company        Company
	CompanyAddress Address
	ClientBilling  Address
	ClientShipping *Address
	Invoice        Invoice
	Items          []InvoiceItem
	Bank           CompanyBankDetails
}

type Company struct {
	Name  string
	Email string
	Phone string
}

type Address struct {
	Name    string
	Line1   string
	City    string
	State   string
	Country string
	Zip     string
}

type Invoice struct {
	InvoiceNumber string
	InvoiceDate   string
	DueDate       string
	Subtotal      float64
	Tax           float64
	Total         float64
	Notes         string
	PaymentInfo   string
	PONumber      string
	Number        string
	Terms         string
	AmountPaid    float64
	AmountDue     float64
	TaxRate       float64
	Discount      float64
	// DocType overrides the printed header ("TAX INVOICE" by default) —
	// e.g. "QUOTATION" when this data is used to render an estimate.
	DocType string
}

type InvoiceItem struct {
	Name    string
	Qty     int
	HSNCode string
	Rate    float64
	TaxRate float64
	Total   float64
}

type CompanyBankDetails struct {
	BankName      string
	AccountNumber string
	IFSCCode      string
	Branch        string
}
