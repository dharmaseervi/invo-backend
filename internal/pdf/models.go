package pdf

type InvoicePDFData struct {
	Company        Company
	CompanyAddress Address
	ClientBilling  Address
	ClientShipping *Address
	Invoice        Invoice
	Items          []InvoiceItem
}

type Company struct {
	Name string
}

type Address struct {
	Name    string
	Line1   string
	City    string
	State   string
	Country string
}

type Invoice struct {
	InvoiceNumber string
	InvoiceDate   string
	DueDate       string
	Subtotal      float64
	Tax           float64
	Total         float64
	Notes         string
}

type InvoiceItem struct {
	Name  string
	Qty   int
	Rate  float64
	Total float64
}
