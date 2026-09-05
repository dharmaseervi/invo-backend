package pdf

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/jung-kurt/gofpdf"
)

// ModernInvoiceGenerator — clean, colorful (violet accent) layout matching
// the app's own brand color, less boxy than the Classic/Tally style.
type ModernInvoiceGenerator struct {
	pdf      *gofpdf.Fpdf
	data     InvoicePDFData
	copyType string
}

// Brand violet, matching Color.sAccent in the iOS app (#7C3AED).
const (
	accentR, accentG, accentB = 124, 58, 237
)

func NewModernInvoiceGenerator(data InvoicePDFData, copyType string) *ModernInvoiceGenerator {
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(0, 0, 0)
	pdf.SetAutoPageBreak(true, 15)
	return &ModernInvoiceGenerator{pdf: pdf, data: data, copyType: strings.ToUpper(copyType)}
}

func (g *ModernInvoiceGenerator) Generate() ([]byte, error) {
	pdf := g.pdf

	pdf.SetFooterFunc(func() {
		pdf.SetY(-10)
		pdf.SetFont("Helvetica", "I", 7)
		pdf.SetTextColor(150, 150, 150)
		pdf.CellFormat(pageW, 5,
			fmt.Sprintf("Page %d — %s", pdf.PageNo(), g.data.Company.Name),
			"", 0, "C", false, 0, "")
		pdf.SetTextColor(0, 0, 0)
	})

	pdf.AddPage()

	y := g.drawHeader(marginT)
	y = g.drawCompanyAndInvoice(y)
	y = g.drawPartySection(y)
	y = g.drawItemsTable(y)
	y = g.drawTotalsSection(y)
	g.drawFooter(y)

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (g *ModernInvoiceGenerator) drawHeader(y float64) float64 {
	pdf := g.pdf

	pdf.SetFont("Helvetica", "B", 22)
	pdf.SetTextColor(accentR, accentG, accentB)
	pdf.SetXY(marginL, y)
	pdf.Cell(pageW/2, 12, g.data.Invoice.HeaderTitle("Invoice"))

	pdf.SetFont("Helvetica", "", 9)
	pdf.SetTextColor(140, 140, 140)
	pdf.SetXY(marginL+pageW/2, y+2)
	pdf.CellFormat(pageW/2, 6, g.copyType+" COPY", "", 0, "R", false, 0, "")
	pdf.SetTextColor(0, 0, 0)

	y += 14
	pdf.SetDrawColor(accentR, accentG, accentB)
	pdf.SetLineWidth(0.8)
	pdf.Line(marginL, y, marginL+pageW, y)
	pdf.SetLineWidth(0.2)
	pdf.SetDrawColor(0, 0, 0)

	return y + 6
}

func (g *ModernInvoiceGenerator) drawCompanyAndInvoice(y float64) float64 {
	pdf := g.pdf
	mid := marginL + pageW/2

	pdf.SetFont("Helvetica", "B", 11)
	pdf.SetXY(marginL, y)
	pdf.Cell(pageW/2, 5, g.data.Company.Name)

	pdf.SetFont("Helvetica", "", 8)
	pdf.SetTextColor(110, 110, 110)
	pdf.SetXY(marginL, y+7)
	pdf.MultiCell(pageW/2-4, 4,
		fmt.Sprintf("%s\n%s, %s - %s",
			g.data.CompanyAddress.Line1, g.data.CompanyAddress.City,
			g.data.CompanyAddress.State, g.data.CompanyAddress.Zip),
		"", "L", false)
	if g.data.Company.Phone != "" {
		pdf.SetXY(marginL, y+20)
		pdf.Cell(pageW/2, 4, "Ph: "+g.data.Company.Phone)
	}
	pdf.SetTextColor(0, 0, 0)

	g.labelValueRight(mid, y, "Invoice No.", g.data.Invoice.InvoiceNumber)
	g.labelValueRight(mid, y+7, "Date", g.data.Invoice.InvoiceDate)
	g.labelValueRight(mid, y+14, "Due Date", g.data.Invoice.DueDate)

	y += 30
	pdf.SetDrawColor(230, 230, 230)
	pdf.Line(marginL, y, marginL+pageW, y)
	pdf.SetDrawColor(0, 0, 0)

	return y + 6
}

func (g *ModernInvoiceGenerator) drawPartySection(y float64) float64 {
	pdf := g.pdf
	mid := marginL + pageW/2

	pdf.SetFont("Helvetica", "B", 8)
	pdf.SetTextColor(accentR, accentG, accentB)
	pdf.SetXY(marginL, y)
	pdf.Cell(60, 4, "BILL TO")

	pdf.SetFont("Helvetica", "B", 9.5)
	pdf.SetTextColor(0, 0, 0)
	pdf.SetXY(marginL, y+6)
	pdf.Cell(pageW/2-4, 4, g.data.ClientBilling.Name)

	pdf.SetFont("Helvetica", "", 8)
	pdf.SetTextColor(110, 110, 110)
	pdf.SetXY(marginL, y+11)
	pdf.MultiCell(pageW/2-4, 3.8,
		fmt.Sprintf("%s, %s, %s - %s",
			g.data.ClientBilling.Line1, g.data.ClientBilling.City,
			g.data.ClientBilling.State, g.data.ClientBilling.Zip),
		"", "L", false)
	pdf.SetTextColor(0, 0, 0)

	pdf.SetFont("Helvetica", "B", 8)
	pdf.SetTextColor(accentR, accentG, accentB)
	pdf.SetXY(mid, y)
	pdf.Cell(60, 4, "SHIP TO")

	if g.data.ClientShipping != nil {
		pdf.SetFont("Helvetica", "B", 9.5)
		pdf.SetTextColor(0, 0, 0)
		pdf.SetXY(mid, y+6)
		pdf.Cell(pageW/2-4, 4, g.data.ClientShipping.Name)
		pdf.SetFont("Helvetica", "", 8)
		pdf.SetTextColor(110, 110, 110)
		pdf.SetXY(mid, y+11)
		pdf.MultiCell(pageW/2-4, 3.8,
			fmt.Sprintf("%s, %s, %s",
				g.data.ClientShipping.Line1, g.data.ClientShipping.City, g.data.ClientShipping.State),
			"", "L", false)
	} else {
		pdf.SetFont("Helvetica", "I", 8)
		pdf.SetTextColor(160, 160, 160)
		pdf.SetXY(mid, y+6)
		pdf.Cell(pageW/2-4, 4, "Same as billing address")
	}
	pdf.SetTextColor(0, 0, 0)

	y += 28
	pdf.SetDrawColor(230, 230, 230)
	pdf.Line(marginL, y, marginL+pageW, y)
	pdf.SetDrawColor(0, 0, 0)

	return y + 6
}

func (g *ModernInvoiceGenerator) drawItemsTable(y float64) float64 {
	pdf := g.pdf

	wNo, wDesc, wHSN, wQty, wRate, wTax, wAmt := 8.0, 62.0, 20.0, 15.0, 28.0, 18.0, 39.0
	rowH, hdrH := 7.0, 8.0

	drawHeader := func(startY float64) {
		pdf.SetFillColor(accentR, accentG, accentB)
		pdf.SetTextColor(255, 255, 255)
		pdf.SetFont("Helvetica", "B", 7.5)
		pdf.SetXY(marginL, startY)
		pdf.CellFormat(wNo, hdrH, "#", "", 0, "C", true, 0, "")
		pdf.CellFormat(wDesc, hdrH, "DESCRIPTION", "", 0, "L", true, 0, "")
		pdf.CellFormat(wHSN, hdrH, "HSN/SAC", "", 0, "C", true, 0, "")
		pdf.CellFormat(wQty, hdrH, "QTY", "", 0, "C", true, 0, "")
		pdf.CellFormat(wRate, hdrH, "RATE", "", 0, "R", true, 0, "")
		pdf.CellFormat(wTax, hdrH, "TAX", "", 0, "C", true, 0, "")
		pdf.CellFormat(wAmt, hdrH, "AMOUNT", "", 1, "R", true, 0, "")
		pdf.SetTextColor(0, 0, 0)
	}

	drawHeader(y)
	pdf.SetFont("Helvetica", "", 8)

	for i, item := range g.data.Items {
		if pdf.GetY() > 250 {
			pdf.AddPage()
			drawHeader(marginT + 10)
			pdf.SetFont("Helvetica", "", 8)
		}
		rowY := pdf.GetY()
		if i%2 == 0 {
			pdf.SetFillColor(248, 245, 253)
			pdf.Rect(marginL, rowY, pageW, rowH, "F")
		}
		pdf.SetXY(marginL, rowY)
		pdf.CellFormat(wNo, rowH, fmt.Sprintf("%d", i+1), "", 0, "C", false, 0, "")
		pdf.CellFormat(wDesc, rowH, item.Name, "", 0, "L", false, 0, "")
		pdf.CellFormat(wHSN, rowH, item.HSNCode, "", 0, "C", false, 0, "")
		pdf.CellFormat(wQty, rowH, fmt.Sprintf("%d", item.Qty), "", 0, "C", false, 0, "")
		pdf.CellFormat(wRate, rowH, fmt.Sprintf("%.2f", item.Rate), "", 0, "R", false, 0, "")
		pdf.CellFormat(wTax, rowH, fmt.Sprintf("%.1f%%", item.TaxRate), "", 0, "C", false, 0, "")
		pdf.CellFormat(wAmt, rowH, fmt.Sprintf("%.2f", item.Total), "", 1, "R", false, 0, "")
	}

	endY := pdf.GetY()
	pdf.SetDrawColor(accentR, accentG, accentB)
	pdf.SetLineWidth(0.5)
	pdf.Line(marginL, endY, marginL+pageW, endY)
	pdf.SetLineWidth(0.2)
	pdf.SetDrawColor(0, 0, 0)

	return endY + 6
}

func (g *ModernInvoiceGenerator) drawTotalsSection(y float64) float64 {
	pdf := g.pdf

	cgst := g.data.Invoice.Tax / 2
	sgst := g.data.Invoice.Tax / 2

	if y > 210 {
		pdf.AddPage()
		y = marginT + 10
	}

	x := marginL + pageW - 80

	g.taxRow(x, y, "Taxable Amount", g.data.Invoice.Subtotal)
	g.taxRow(x, y+6, "CGST @ 9.0%", cgst)
	g.taxRow(x, y+12, "SGST @ 9.0%", sgst)

	y += 20
	pdf.SetFillColor(accentR, accentG, accentB)
	pdf.Rect(x-4, y, 84, 11, "F")
	pdf.SetTextColor(255, 255, 255)
	pdf.SetFont("Helvetica", "B", 10)
	pdf.SetXY(x, y+2.5)
	pdf.Cell(35, 6, "TOTAL")
	pdf.SetXY(x, y+2.5)
	pdf.CellFormat(80, 6, fmt.Sprintf("INR %.2f", g.data.Invoice.Total), "", 0, "R", false, 0, "")
	pdf.SetTextColor(0, 0, 0)

	y2 := y + 15
	pdf.SetFont("Helvetica", "I", 7.5)
	pdf.SetTextColor(110, 110, 110)
	pdf.SetXY(marginL, y2)
	pdf.Cell(pageW, 5, "In Words: "+AmountToWords(g.data.Invoice.Total))
	pdf.SetTextColor(0, 0, 0)

	return y2 + 10
}

func (g *ModernInvoiceGenerator) drawFooter(y float64) float64 {
	pdf := g.pdf
	mid := marginL + pageW/2

	if y > 235 {
		pdf.AddPage()
		y = marginT + 10
	}

	pdf.SetDrawColor(230, 230, 230)
	pdf.Line(marginL, y, marginL+pageW, y)
	pdf.SetDrawColor(0, 0, 0)
	y += 6

	pdf.SetFont("Helvetica", "B", 8)
	pdf.SetTextColor(accentR, accentG, accentB)
	pdf.SetXY(marginL, y)
	pdf.Cell(60, 4, "BANK DETAILS")
	pdf.SetTextColor(0, 0, 0)

	g.bankRow(y+7, "Bank Name", g.data.Bank.BankName)
	g.bankRow(y+12, "Account No.", g.data.Bank.AccountNumber)
	g.bankRow(y+17, "IFSC Code", g.data.Bank.IFSCCode)
	g.bankRow(y+22, "Branch", g.data.Bank.Branch)

	pdf.SetFont("Helvetica", "B", 8)
	pdf.SetTextColor(accentR, accentG, accentB)
	pdf.SetXY(mid, y)
	pdf.Cell(pageW/2, 4, "FOR "+strings.ToUpper(g.data.Company.Name))
	pdf.SetTextColor(0, 0, 0)

	pdf.SetDrawColor(150, 150, 150)
	pdf.Line(mid+15, y+26, marginL+pageW-5, y+26)
	pdf.SetDrawColor(0, 0, 0)

	pdf.SetFont("Helvetica", "", 7)
	pdf.SetTextColor(120, 120, 120)
	pdf.SetXY(mid+15, y+27)
	pdf.CellFormat(pageW/2-20, 4, "Authorised Signatory", "", 0, "C", false, 0, "")
	pdf.SetTextColor(0, 0, 0)

	y2 := y + 32
	pdf.SetFont("Helvetica", "I", 6.5)
	pdf.SetTextColor(140, 140, 140)
	pdf.SetXY(marginL, y2)
	pdf.MultiCell(pageW, 3.5,
		"Declaration: We declare that this invoice shows the actual price of the goods "+
			"described and that all particulars are true and correct. Goods once sold will not be taken back.",
		"", "L", false)
	pdf.SetTextColor(0, 0, 0)

	return y2 + 8
}

func (g *ModernInvoiceGenerator) labelValueRight(x, y float64, label, value string) {
	pdf := g.pdf
	pdf.SetFont("Helvetica", "", 8)
	pdf.SetTextColor(140, 140, 140)
	pdf.SetXY(x, y)
	pdf.CellFormat(pageW/2, 4, label, "", 0, "R", false, 0, "")
	pdf.SetFont("Helvetica", "B", 8.5)
	pdf.SetTextColor(0, 0, 0)
	pdf.SetXY(x, y+4)
	pdf.CellFormat(pageW/2, 4, value, "", 0, "R", false, 0, "")
}

func (g *ModernInvoiceGenerator) taxRow(x, y float64, label string, value float64) {
	pdf := g.pdf
	pdf.SetFont("Helvetica", "", 8)
	pdf.SetTextColor(110, 110, 110)
	pdf.SetXY(x, y)
	pdf.Cell(45, 5, label)
	pdf.SetFont("Helvetica", "B", 8)
	pdf.SetTextColor(0, 0, 0)
	pdf.SetXY(x+45, y)
	pdf.CellFormat(35, 5, fmt.Sprintf("%.2f", value), "", 0, "R", false, 0, "")
}

func (g *ModernInvoiceGenerator) bankRow(y float64, label, value string) {
	pdf := g.pdf
	pdf.SetFont("Helvetica", "", 7.5)
	pdf.SetTextColor(110, 110, 110)
	pdf.SetXY(marginL, y)
	pdf.Cell(24, 4, label+":")
	pdf.SetFont("Helvetica", "B", 7.5)
	pdf.SetTextColor(0, 0, 0)
	pdf.Cell(55, 4, value)
}
