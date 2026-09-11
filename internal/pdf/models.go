package pdf

import "strconv"

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
	// IsInterstate decides CGST+SGST vs IGST. Under Indian GST the place of supply
	// relative to the seller's state determines this, and printing the wrong pair
	// makes the document invalid as a tax invoice — so it is never assumed.
	IsInterstate bool
	// TaxLines is the invoice's tax grouped by GST rate, since one invoice can carry
	// items at 5%, 12% and 18% and each has to be shown at its own rate.
	TaxLines []TaxLine
	// DocType overrides the printed header ("TAX INVOICE" by default) —
	// e.g. "QUOTATION" when this data is used to render an estimate.
	DocType string
}

type TaxLine struct {
	Rate    float64 // the full GST rate for this group, e.g. 18 for 18%
	Taxable float64
	Amount  float64 // tax charged at this rate
}

// TaxableValue is the value GST was actually charged on: the subtotal after any
// invoice-level discount. Derived from the rate-wise lines so the figure printed above
// the CGST/SGST rows is always the base those rows were computed from.
func (i Invoice) TaxableValue() float64 {
	if len(i.TaxLines) == 0 {
		return i.Subtotal - i.Discount
	}
	var sum float64
	for _, line := range i.TaxLines {
		sum += line.Taxable
	}
	return sum
}

// TaxSummaryRow is one printable line of a PDF's tax box.
type TaxSummaryRow struct {
	Label  string
	Amount float64
}

// TaxSummaryRows builds the tax box contents: IGST for an interstate supply, an equal
// CGST/SGST pair otherwise, one entry per distinct GST rate. Falls back to a single
// unlabelled split when rate-wise data isn't available (estimates, legacy invoices).
func (i Invoice) TaxSummaryRows() []TaxSummaryRow {
	pct := func(r float64) string { return strconv.FormatFloat(r, 'f', -1, 64) }

	if len(i.TaxLines) == 0 {
		if i.Tax == 0 {
			return nil
		}
		if i.IsInterstate {
			return []TaxSummaryRow{{"IGST", i.Tax}}
		}
		return []TaxSummaryRow{{"CGST", i.Tax / 2}, {"SGST", i.Tax / 2}}
	}

	rows := make([]TaxSummaryRow, 0, len(i.TaxLines)*2)
	for _, line := range i.TaxLines {
		if line.Amount == 0 {
			continue
		}
		if i.IsInterstate {
			rows = append(rows, TaxSummaryRow{"IGST @ " + pct(line.Rate) + "%", line.Amount})
			continue
		}
		half := pct(line.Rate / 2)
		rows = append(rows,
			TaxSummaryRow{"CGST @ " + half + "%", line.Amount / 2},
			TaxSummaryRow{"SGST @ " + half + "%", line.Amount / 2},
		)
	}
	return rows
}

type InvoiceItem struct {
	Name    string
	Qty     int
	HSNCode string
	Rate    float64
	TaxRate float64
	// Total is the line value including its own GST, as stored on the invoice.
	Total float64
	// Taxable is that line net of GST — the figure a tax invoice's amount column has
	// to show, so the line amounts add up to the taxable total the tax is charged on.
	Taxable float64
}

type CompanyBankDetails struct {
	BankName      string
	AccountNumber string
	IFSCCode      string
	Branch        string
}
