package pdf

import (
	"bytes"
	"fmt"
	"math"
	"strings"

	"github.com/jung-kurt/gofpdf"
)

type TallyInvoiceGenerator struct {
	pdf      *gofpdf.Fpdf
	data     InvoicePDFData
	copyType string
}

func NewTallyInvoiceGenerator(data InvoicePDFData, copyType string) *TallyInvoiceGenerator {
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(0, 0, 0)
	pdf.SetAutoPageBreak(true, 15)
	return &TallyInvoiceGenerator{
		pdf:      pdf,
		data:     data,
		copyType: strings.ToUpper(copyType),
	}
}

const (
	marginL = 10.0
	marginT = 10.0
	pageW   = 190.0
	pageH   = 277.0
)

// ─── Amount To Words ─────────────────────────────────────────────────────────
func AmountToWords(n float64) string {
	ones := []string{"", "One", "Two", "Three", "Four", "Five", "Six", "Seven",
		"Eight", "Nine", "Ten", "Eleven", "Twelve", "Thirteen", "Fourteen",
		"Fifteen", "Sixteen", "Seventeen", "Eighteen", "Nineteen"}
	tens := []string{"", "", "Twenty", "Thirty", "Forty", "Fifty",
		"Sixty", "Seventy", "Eighty", "Ninety"}

	var convert func(int) string
	convert = func(n int) string {
		switch {
		case n == 0:
			return ""
		case n < 20:
			return ones[n] + " "
		case n < 100:
			return tens[n/10] + " " + convert(n%10)
		case n < 1000:
			return ones[n/100] + " Hundred " + convert(n%100)
		case n < 100000:
			return convert(n/1000) + "Thousand " + convert(n%1000)
		case n < 10000000:
			return convert(n/100000) + "Lakh " + convert(n%100000)
		default:
			return convert(n/10000000) + "Crore " + convert(n%10000000)
		}
	}

	intPart := int(math.Abs(n))
	fracPart := int(math.Round((math.Abs(n) - float64(intPart)) * 100))
	if intPart == 0 {
		return "Zero Rupees Only"
	}
	result := "Rupees " + strings.TrimSpace(convert(intPart))
	if fracPart > 0 {
		result += " and " + strings.TrimSpace(convert(fracPart)) + " Paise"
	}
	return result + " Only"
}

// ─── Generate ────────────────────────────────────────────────────────────────
func (g *TallyInvoiceGenerator) Generate() ([]byte, error) {

	// Repeat header on every new page
	g.pdf.SetHeaderFunc(func() {
		if g.pdf.PageNo() > 1 {
			g.pdf.SetFont("Helvetica", "B", 9)
			g.pdf.SetFillColor(20, 20, 20)
			g.pdf.SetTextColor(255, 255, 255)
			g.pdf.Rect(marginL, marginT, pageW, 8, "F")
			g.pdf.SetXY(marginL, marginT)
			g.pdf.CellFormat(pageW, 8,
				fmt.Sprintf("TAX INVOICE - %s (Continued...)", g.data.Invoice.InvoiceNumber),
				"", 1, "C", false, 0, "")
			g.pdf.SetTextColor(0, 0, 0)
		}
	})

	// Page number footer
	g.pdf.SetFooterFunc(func() {
		g.pdf.SetY(-10)
		g.pdf.SetFont("Helvetica", "I", 7)
		g.pdf.SetTextColor(150, 150, 150)
		g.pdf.CellFormat(pageW, 5,
			fmt.Sprintf("Page %d — %s", g.pdf.PageNo(), g.data.Company.Name),
			"", 0, "C", false, 0, "")
		g.pdf.SetTextColor(0, 0, 0)
	})

	g.pdf.AddPage()

	y := marginT
	y = g.drawHeader(y)
	y = g.drawCompanyAndInvoice(y)
	y = g.drawPartySection(y)
	y = g.drawItemsTable(y)
	y = g.drawTotalsSection(y)
	g.drawFooter(y)

	var buf bytes.Buffer
	if err := g.pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// ─── Header ──────────────────────────────────────────────────────────────────
func (g *TallyInvoiceGenerator) drawHeader(y float64) float64 {
	pdf := g.pdf
	h := 10.0

	pdf.SetFillColor(20, 20, 20)
	pdf.Rect(marginL, y, pageW, h, "F")

	pdf.SetFont("Helvetica", "B", 12)
	pdf.SetTextColor(255, 255, 255)
	pdf.SetXY(marginL, y)
	pdf.CellFormat(pageW, h, "TAX INVOICE", "", 0, "C", false, 0, "")

	pdf.SetFont("Helvetica", "", 7)
	pdf.SetTextColor(180, 180, 180)
	pdf.SetXY(marginL, y+1)
	pdf.CellFormat(pageW-3, h-2, g.copyType+" COPY", "", 0, "R", false, 0, "")

	pdf.SetTextColor(0, 0, 0)
	return y + h
}

// ─── Company + Invoice Details ───────────────────────────────────────────────
func (g *TallyInvoiceGenerator) drawCompanyAndInvoice(y float64) float64 {
	pdf := g.pdf
	h := 32.0
	mid := marginL + pageW/2

	// Company name
	pdf.SetFont("Helvetica", "B", 10)
	pdf.SetXY(marginL+2, y+3)
	pdf.Cell(pageW/2-4, 5, g.data.Company.Name)

	// Company address
	pdf.SetFont("Helvetica", "", 8)
	pdf.SetTextColor(60, 60, 60)
	pdf.SetXY(marginL+2, y+9)
	pdf.MultiCell(pageW/2-4, 4,
		fmt.Sprintf("%s\n%s, %s - %s",
			g.data.CompanyAddress.Line1,
			g.data.CompanyAddress.City,
			g.data.CompanyAddress.State,
			g.data.CompanyAddress.Zip,
		), "", "L", false)

	// Phone
	if g.data.Company.Phone != "" {
		pdf.SetFont("Helvetica", "", 7.5)
		pdf.SetXY(marginL+2, y+22)
		pdf.Cell(pageW/2-4, 4, "Ph: "+g.data.Company.Phone)
	}
	pdf.SetTextColor(0, 0, 0)

	// Vertical divider
	pdf.SetDrawColor(200, 200, 200)
	pdf.Line(mid, y, mid, y+h)
	pdf.SetDrawColor(0, 0, 0)

	// Invoice details
	g.labelValue(mid+3, y+4, "Invoice No.", g.data.Invoice.InvoiceNumber)
	g.labelValue(mid+3, y+11, "Date", g.data.Invoice.InvoiceDate)
	g.labelValue(mid+3, y+18, "Due Date", g.data.Invoice.DueDate)
	if g.data.Invoice.PONumber != "" {
		g.labelValue(mid+3, y+25, "PO Number", g.data.Invoice.PONumber)
	}

	// Bottom border
	pdf.SetDrawColor(0, 0, 0)
	pdf.Line(marginL, y+h, marginL+pageW, y+h)

	return y + h
}

// ─── Party Section ───────────────────────────────────────────────────────────
func (g *TallyInvoiceGenerator) drawPartySection(y float64) float64 {
	pdf := g.pdf
	h := 28.0
	mid := marginL + pageW/2

	// BILL TO
	pdf.SetFillColor(240, 240, 240)
	pdf.Rect(marginL, y, pageW/2, 6, "F")
	pdf.SetFont("Helvetica", "B", 7.5)
	pdf.SetXY(marginL+2, y+1)
	pdf.Cell(40, 4, "BILL TO")

	pdf.SetFont("Helvetica", "B", 8.5)
	pdf.SetXY(marginL+2, y+8)
	pdf.Cell(pageW/2-4, 4, g.data.ClientBilling.Name)

	pdf.SetFont("Helvetica", "", 7.5)
	pdf.SetTextColor(60, 60, 60)
	pdf.SetXY(marginL+2, y+13)
	pdf.MultiCell(pageW/2-4, 3.8,
		fmt.Sprintf("%s, %s, %s - %s",
			g.data.ClientBilling.Line1,
			g.data.ClientBilling.City,
			g.data.ClientBilling.State,
			g.data.ClientBilling.Zip,
		), "", "L", false)
	pdf.SetTextColor(0, 0, 0)

	// Vertical divider
	pdf.SetDrawColor(200, 200, 200)
	pdf.Line(mid, y, mid, y+h)
	pdf.SetDrawColor(0, 0, 0)

	// SHIP TO
	pdf.SetFillColor(240, 240, 240)
	pdf.Rect(mid, y, pageW/2, 6, "F")
	pdf.SetFont("Helvetica", "B", 7.5)
	pdf.SetXY(mid+3, y+1)
	pdf.Cell(40, 4, "SHIP TO")

	if g.data.ClientShipping != nil {
		pdf.SetFont("Helvetica", "B", 8.5)
		pdf.SetXY(mid+3, y+8)
		pdf.Cell(pageW/2-6, 4, g.data.ClientShipping.Name)
		pdf.SetFont("Helvetica", "", 7.5)
		pdf.SetTextColor(60, 60, 60)
		pdf.SetXY(mid+3, y+13)
		pdf.MultiCell(pageW/2-6, 3.8,
			fmt.Sprintf("%s, %s, %s",
				g.data.ClientShipping.Line1,
				g.data.ClientShipping.City,
				g.data.ClientShipping.State,
			), "", "L", false)
		pdf.SetTextColor(0, 0, 0)
	} else {
		pdf.SetFont("Helvetica", "I", 7.5)
		pdf.SetTextColor(150, 150, 150)
		pdf.SetXY(mid+3, y+10)
		pdf.Cell(pageW/2-6, 4, "Same as billing address")
		pdf.SetTextColor(0, 0, 0)
	}

	// Bottom border
	pdf.SetDrawColor(0, 0, 0)
	pdf.Line(marginL, y+h, marginL+pageW, y+h)

	return y + h
}

// ─── Items Table ─────────────────────────────────────────────────────────────
func (g *TallyInvoiceGenerator) drawItemsTable(y float64) float64 {
	pdf := g.pdf

	wNo := 8.0
	wDesc := 62.0
	wHSN := 20.0
	wQty := 15.0
	wRate := 28.0
	wTax := 18.0
	wAmt := 39.0
	rowH := 6.5
	hdrH := 7.0

	// Header draw function — reused on new pages
	drawTableHeader := func(startY float64) {
		pdf.SetFillColor(20, 20, 20)
		pdf.SetTextColor(255, 255, 255)
		pdf.SetFont("Helvetica", "B", 7.5)
		pdf.SetXY(marginL, startY)
		pdf.CellFormat(wNo, hdrH, "#", "R", 0, "C", true, 0, "")
		pdf.CellFormat(wDesc, hdrH, "DESCRIPTION", "R", 0, "L", true, 0, "")
		pdf.CellFormat(wHSN, hdrH, "HSN/SAC", "R", 0, "C", true, 0, "")
		pdf.CellFormat(wQty, hdrH, "QTY", "R", 0, "C", true, 0, "")
		pdf.CellFormat(wRate, hdrH, "RATE (INR)", "R", 0, "R", true, 0, "")
		pdf.CellFormat(wTax, hdrH, "TAX %", "R", 0, "C", true, 0, "")
		pdf.CellFormat(wAmt, hdrH, "AMOUNT (INR)", "", 1, "R", true, 0, "")
		pdf.SetTextColor(0, 0, 0)
	}

	drawTableHeader(y)
	pdf.SetFont("Helvetica", "", 8)

	for i, item := range g.data.Items {
		// Page break check — leave space for totals/footer
		if pdf.GetY() > 250 {
			pdf.AddPage()
			drawTableHeader(marginT + 10)
			pdf.SetFont("Helvetica", "", 8)
		}

		rowY := pdf.GetY()

		// Alternate row shading
		if i%2 == 0 {
			pdf.SetFillColor(248, 248, 248)
			pdf.Rect(marginL, rowY, pageW, rowH, "F")
		}

		pdf.SetXY(marginL, rowY)
		pdf.CellFormat(wNo, rowH, fmt.Sprintf("%d", i+1), "R", 0, "C", false, 0, "")
		pdf.CellFormat(wDesc, rowH, item.Name, "R", 0, "L", false, 0, "")
		pdf.CellFormat(wHSN, rowH, item.HSNCode, "R", 0, "C", false, 0, "")
		pdf.CellFormat(wQty, rowH, fmt.Sprintf("%d", item.Qty), "R", 0, "C", false, 0, "")
		pdf.CellFormat(wRate, rowH, fmt.Sprintf("%.2f", item.Rate), "R", 0, "R", false, 0, "")
		pdf.CellFormat(wTax, rowH, fmt.Sprintf("%.1f%%", item.TaxRate), "R", 0, "C", false, 0, "")
		pdf.CellFormat(wAmt, rowH, fmt.Sprintf("%.2f", item.Total), "", 1, "R", false, 0, "")
	}

	endY := pdf.GetY()
	pdf.SetDrawColor(0, 0, 0)
	pdf.Line(marginL, endY, marginL+pageW, endY)

	return endY
}

// ─── Totals Section ──────────────────────────────────────────────────────────
func (g *TallyInvoiceGenerator) drawTotalsSection(y float64) float64 {
	pdf := g.pdf
	h := 38.0
	mid := marginL + pageW/2

	cgst := g.data.Invoice.Tax / 2
	sgst := g.data.Invoice.Tax / 2

	// If not enough space for totals add new page
	if y > 220 {
		pdf.AddPage()
		y = marginT + 10
	}

	// GST Summary (left)
	pdf.SetFillColor(240, 240, 240)
	pdf.Rect(marginL, y, pageW/2, 6, "F")
	pdf.SetFont("Helvetica", "B", 7.5)
	pdf.SetXY(marginL+2, y+1)
	pdf.Cell(40, 4, "GST SUMMARY")

	g.taxRow(marginL+2, y+8, "Taxable Amount", g.data.Invoice.Subtotal)
	g.taxRow(marginL+2, y+14, "CGST @ 9.0%", cgst)
	g.taxRow(marginL+2, y+20, "SGST @ 9.0%", sgst)
	g.taxRow(marginL+2, y+26, "Total Tax", g.data.Invoice.Tax)

	// Vertical divider
	pdf.SetDrawColor(200, 200, 200)
	pdf.Line(mid, y, mid, y+h)
	pdf.SetDrawColor(0, 0, 0)

	// Invoice Summary (right)
	pdf.SetFillColor(240, 240, 240)
	pdf.Rect(mid, y, pageW/2, 6, "F")
	pdf.SetFont("Helvetica", "B", 7.5)
	pdf.SetXY(mid+3, y+1)
	pdf.Cell(40, 4, "INVOICE SUMMARY")

	g.taxRow(mid+3, y+8, "Subtotal", g.data.Invoice.Subtotal)
	g.taxRow(mid+3, y+14, "Total Tax", g.data.Invoice.Tax)

	// Grand Total bar
	pdf.SetFillColor(20, 20, 20)
	pdf.Rect(mid, y+22, pageW/2, 9, "F")
	pdf.SetTextColor(255, 255, 255)
	pdf.SetFont("Helvetica", "B", 9)
	pdf.SetXY(mid+3, y+23)
	pdf.Cell(30, 7, "GRAND TOTAL")
	pdf.SetXY(mid+3, y+23)
	pdf.CellFormat(pageW/2-6, 7,
		fmt.Sprintf("INR %.2f", g.data.Invoice.Total),
		"", 0, "R", false, 0, "")
	pdf.SetTextColor(0, 0, 0)

	// Amount in words
	y2 := y + h
	pdf.SetFillColor(245, 245, 245)
	pdf.Rect(marginL, y2, pageW, 8, "F")
	pdf.SetFont("Helvetica", "I", 7.5)
	pdf.SetTextColor(40, 40, 40)
	pdf.SetXY(marginL+2, y2+2)
	pdf.Cell(pageW-4, 5, "In Words: "+AmountToWords(g.data.Invoice.Total))
	pdf.SetTextColor(0, 0, 0)

	pdf.SetDrawColor(0, 0, 0)
	pdf.Line(marginL, y2+8, marginL+pageW, y2+8)

	return y2 + 8
}

// ─── Footer ──────────────────────────────────────────────────────────────────
func (g *TallyInvoiceGenerator) drawFooter(y float64) float64 {
	pdf := g.pdf
	h := 35.0
	mid := marginL + pageW/2

	// If not enough space add new page
	if y > 230 {
		pdf.AddPage()
		y = marginT + 10
	}

	// Bank Details (left)
	pdf.SetFillColor(240, 240, 240)
	pdf.Rect(marginL, y, pageW/2, 6, "F")
	pdf.SetFont("Helvetica", "B", 7.5)
	pdf.SetXY(marginL+2, y+1)
	pdf.Cell(40, 4, "BANK DETAILS")

	g.bankRow(y+9, "Bank Name", g.data.Bank.BankName)
	g.bankRow(y+14, "Account No.", g.data.Bank.AccountNumber)
	g.bankRow(y+19, "IFSC Code", g.data.Bank.IFSCCode)
	g.bankRow(y+24, "Branch", g.data.Bank.Branch)

	// Vertical divider
	pdf.SetDrawColor(200, 200, 200)
	pdf.Line(mid, y, mid, y+h)
	pdf.SetDrawColor(0, 0, 0)

	// Signature (right)
	pdf.SetFillColor(240, 240, 240)
	pdf.Rect(mid, y, pageW/2, 6, "F")
	pdf.SetFont("Helvetica", "B", 7.5)
	pdf.SetXY(mid+3, y+1)
	pdf.Cell(pageW/2-6, 4, "FOR "+strings.ToUpper(g.data.Company.Name))

	// Signature line
	pdf.SetDrawColor(120, 120, 120)
	pdf.Line(mid+15, y+29, marginL+pageW-5, y+29)
	pdf.SetDrawColor(0, 0, 0)

	pdf.SetFont("Helvetica", "", 7)
	pdf.SetTextColor(100, 100, 100)
	pdf.SetXY(mid+3, y+30)
	pdf.CellFormat(pageW/2-6, 4, "Authorised Signatory", "", 0, "C", false, 0, "")
	pdf.SetTextColor(0, 0, 0)

	// Bottom line
	pdf.Line(marginL, y+h, marginL+pageW, y+h)

	// Declaration
	y2 := y + h + 2
	pdf.SetFont("Helvetica", "I", 6.5)
	pdf.SetTextColor(110, 110, 110)
	pdf.SetXY(marginL+2, y2)
	pdf.MultiCell(pageW-4, 3.5,
		"Declaration: We declare that this invoice shows the actual price of the goods "+
			"described and that all particulars are true and correct. "+
			"Goods once sold will not be taken back.",
		"", "L", false)
	pdf.SetTextColor(0, 0, 0)

	return y2 + 8
}

// ─── Helpers ─────────────────────────────────────────────────────────────────
func (g *TallyInvoiceGenerator) labelValue(x, y float64, label, value string) {
	pdf := g.pdf
	pdf.SetFont("Helvetica", "", 7.5)
	pdf.SetTextColor(100, 100, 100)
	pdf.SetXY(x, y)
	pdf.Cell(22, 4, label+":")
	pdf.SetFont("Helvetica", "B", 7.5)
	pdf.SetTextColor(0, 0, 0)
	pdf.Cell(55, 4, value)
}

func (g *TallyInvoiceGenerator) taxRow(x, y float64, label string, value float64) {
	pdf := g.pdf
	pdf.SetFont("Helvetica", "", 7.5)
	pdf.SetTextColor(70, 70, 70)
	pdf.SetXY(x, y)
	pdf.Cell(38, 5, label)
	pdf.SetFont("Helvetica", "B", 7.5)
	pdf.SetTextColor(0, 0, 0)
	pdf.SetXY(x+38, y)
	pdf.CellFormat(42, 5, fmt.Sprintf("%.2f", value), "", 0, "R", false, 0, "")
}

func (g *TallyInvoiceGenerator) bankRow(y float64, label, value string) {
	pdf := g.pdf
	pdf.SetFont("Helvetica", "", 7.5)
	pdf.SetTextColor(100, 100, 100)
	pdf.SetXY(marginL+2, y)
	pdf.Cell(24, 4, label+":")
	pdf.SetFont("Helvetica", "B", 7.5)
	pdf.SetTextColor(0, 0, 0)
	pdf.Cell(55, 4, value)
}

// ─── Global Wrapper ───────────────────────────────────────────────────────────
func GenerateTallyInvoicePDF(data InvoicePDFData, copyType string) ([]byte, error) {
	generator := NewTallyInvoiceGenerator(data, copyType)
	return generator.Generate()
}
