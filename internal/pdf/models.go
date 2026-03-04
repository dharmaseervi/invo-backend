package pdf

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
}

type InvoiceItem struct {
	Name  string
	Qty   int
	Rate  float64
	Total float64
}

type CompanyBankDetails struct {
	BankName      string
	AccountNumber string
	IFSCCode      string
	Branch        string
}
