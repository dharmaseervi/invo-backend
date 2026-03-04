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
	pdf.SetMargins(10, 10, 10)
	pdf.SetAutoPageBreak(true, 10)
	return &TallyInvoiceGenerator{
		pdf:      pdf,
		data:     data,
		copyType: strings.ToUpper(copyType),
	}
}

// AmountToWords converts float64 to formal currency string
func AmountToWords(n float64) string {
	units := []string{"", "One", "Two", "Three", "Four", "Five", "Six", "Seven", "Eight", "Nine", "Ten", "Eleven", "Twelve", "Thirteen", "Fourteen", "Fifteen", "Sixteen", "Seventeen", "Eighteen", "Nineteen"}
	tens := []string{"", "", "Twenty", "Thirty", "Forty", "Fifty", "Sixty", "Seventy", "Eighty", "Ninety"}
	thousands := []string{"", "Thousand", "Million", "Billion"}

	var convertSection func(int) string
	convertSection = func(val int) string {
		res := ""
		if val >= 100 {
			res += units[val/100] + " Hundred "
			val %= 100
		}
		if val >= 20 {
			res += tens[val/10] + " "
			val %= 10
		}
		if val > 0 {
			res += units[val] + " "
		}
		return res
	}

	intPart := int(math.Abs(n))
	fracPart := int(math.Round((math.Abs(n) - float64(intPart)) * 100))

	if intPart == 0 {
		return "Zero Dollars Only"
	}

	wordResult := ""
	for i := 0; intPart > 0; i++ {
		if intPart%1000 != 0 {
			wordResult = convertSection(intPart%1000) + thousands[i] + " " + wordResult
		}
		intPart /= 1000
	}

	final := strings.TrimSpace(wordResult) + " Dollars"
	if fracPart > 0 {
		final += " and " + convertSection(fracPart) + " Cents"
	}
	return final + " Only"
}

func (g *TallyInvoiceGenerator) Generate() ([]byte, error) {
	g.pdf.AddPage()
	g.pdf.SetFont("Helvetica", "B", 10)

	// Outer Border for the whole page (Tally style)
	pageW, pageH := 190.0, 277.0
	g.pdf.Rect(10, 10, pageW, pageH, "D")

	// 1. Header Title
	g.pdf.SetXY(10, 10)
	g.pdf.CellFormat(pageW, 7, "TAX INVOICE", "B", 1, "C", false, 0, "")

	// 2. Top Section: Company & Invoice Details
	midpoint := 10 + (pageW / 2)
	currentY := g.pdf.GetY()

	// Left: Company Info
	g.pdf.SetFont("Helvetica", "B", 9)
	g.pdf.SetXY(12, currentY+2)
	g.pdf.Cell(pageW/2, 4, g.data.Company.Name)
	g.pdf.SetFont("Helvetica", "", 8)
	g.pdf.SetXY(12, currentY+6)
	g.pdf.MultiCell(pageW/2-5, 4, fmt.Sprintf("%s\n%s, %s %s", g.data.CompanyAddress.Line1, g.data.CompanyAddress.City, g.data.CompanyAddress.State, g.data.CompanyAddress.Zip), "", "L", false)

	// Middle Vertical Line
	g.pdf.Line(midpoint, currentY, midpoint, currentY+35)

	// Right: Invoice Details
	g.pdf.SetXY(midpoint+2, currentY+2)
	g.pdf.SetFont("Helvetica", "", 8)
	g.pdf.Cell(25, 4, "Invoice No:")
	g.pdf.SetFont("Helvetica", "B", 8)
	g.pdf.Cell(40, 4, g.data.Invoice.InvoiceNumber)

	g.pdf.SetXY(midpoint+2, currentY+10)
	g.pdf.SetFont("Helvetica", "", 8)
	g.pdf.Cell(25, 4, "Date:")
	g.pdf.SetFont("Helvetica", "B", 8)
	g.pdf.Cell(40, 4, g.data.Invoice.InvoiceDate)

	g.pdf.SetXY(10, currentY+35)
	g.pdf.CellFormat(pageW, 0, "", "T", 1, "", false, 0, "")

	// 3. Billing Section
	currentY = g.pdf.GetY()
	g.pdf.SetXY(12, currentY+2)
	g.pdf.SetFont("Helvetica", "I", 8)
	g.pdf.Cell(40, 4, "Buyer:")
	g.pdf.SetFont("Helvetica", "B", 9)
	g.pdf.SetXY(12, currentY+6)
	g.pdf.Cell(80, 4, g.data.ClientBilling.Name)
	g.pdf.SetFont("Helvetica", "", 8)
	g.pdf.SetXY(12, currentY+10)
	g.pdf.MultiCell(80, 4, fmt.Sprintf("%s\n%s, %s %s", g.data.ClientBilling.Line1, g.data.ClientBilling.City, g.data.ClientBilling.State, g.data.ClientBilling.Zip), "", "L", false)

	g.pdf.SetXY(10, currentY+30)
	g.pdf.CellFormat(pageW, 0, "", "T", 1, "", false, 0, "")

	// 4. Items Table
	g.drawTallyTable()

	// 5. Footer (Amount in Words & Signature)
	g.drawTallyFooter()

	var buf bytes.Buffer
	if err := g.pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (g *TallyInvoiceGenerator) drawTallyTable() {
	// x, y := 10.0, g.pdf.GetY()
	wSl, wDesc, wQty, wRate, wAmt := 12.0, 93.0, 20.0, 30.0, 35.0

	// Table Header
	g.pdf.SetFont("Helvetica", "B", 8)
	g.pdf.CellFormat(wSl, 7, "SI No.", "RB", 0, "C", false, 0, "")
	g.pdf.CellFormat(wDesc, 7, "Description of Goods", "RB", 0, "C", false, 0, "")
	g.pdf.CellFormat(wQty, 7, "Quantity", "RB", 0, "C", false, 0, "")
	g.pdf.CellFormat(wRate, 7, "Rate", "RB", 0, "C", false, 0, "")
	g.pdf.CellFormat(wAmt, 7, "Amount", "B", 1, "C", false, 0, "")

	// Vertical lines for the grid body
	tableStartY := g.pdf.GetY()
	tableHeight := 100.0

	g.pdf.SetFont("Helvetica", "", 8)
	for i, item := range g.data.Items {
		g.pdf.CellFormat(wSl, 6, fmt.Sprintf("%d", i+1), "R", 0, "C", false, 0, "")
		g.pdf.CellFormat(wDesc, 6, item.Name, "R", 0, "L", false, 0, "")
		g.pdf.CellFormat(wQty, 6, fmt.Sprintf("%d", item.Qty), "R", 0, "C", false, 0, "")
		g.pdf.CellFormat(wRate, 6, fmt.Sprintf("%.2f", item.Rate), "R", 0, "R", false, 0, "")
		g.pdf.CellFormat(wAmt, 6, fmt.Sprintf("%.2f", item.Total), "", 1, "R", false, 0, "")
	}

	// Extend the lines to the bottom of the table
	// endY := g.pdf.GetY()
	g.pdf.Line(10+wSl, tableStartY, 10+wSl, tableStartY+tableHeight)
	g.pdf.Line(10+wSl+wDesc, tableStartY, 10+wSl+wDesc, tableStartY+tableHeight)
	g.pdf.Line(10+wSl+wDesc+wQty, tableStartY, 10+wSl+wDesc+wQty, tableStartY+tableHeight)
	g.pdf.Line(10+wSl+wDesc+wQty+wRate, tableStartY, 10+wSl+wDesc+wQty+wRate, tableStartY+tableHeight)

	// Bottom of Table
	g.pdf.SetXY(10, tableStartY+tableHeight)
	g.pdf.CellFormat(wSl+wDesc+wQty+wRate, 7, "Total", "TR", 0, "R", false, 0, "")
	g.pdf.CellFormat(wAmt, 7, fmt.Sprintf("%.2f", g.data.Invoice.Total), "T", 1, "R", false, 0, "")
}

func (g *TallyInvoiceGenerator) drawTallyFooter() {
	y := g.pdf.GetY()

	// Amount in Words Section
	g.pdf.Rect(10, y, 190, 10, "D")
	g.pdf.SetFont("Helvetica", "", 7)
	g.pdf.SetXY(12, y+1)
	g.pdf.Cell(40, 4, "Amount Chargeable (in words)")
	g.pdf.SetFont("Helvetica", "B", 8)
	g.pdf.SetXY(12, y+5)
	g.pdf.Cell(180, 4, AmountToWords(g.data.Invoice.Total))

	y += 10
	// --- Left Section: Bank Details ---
	g.pdf.Rect(10, y, 95, 35, "D")
	g.pdf.SetFont("Helvetica", "B", 7)
	g.pdf.SetXY(12, y+2)
	g.pdf.Cell(40, 3, "Company's Bank Details:")

	g.pdf.SetFont("Helvetica", "", 8)
	bankDetailsY := y + 7
	details := []struct {
		label string
		value string
	}{
		{"Bank Name", g.data.Bank.BankName},
		{"A/c No.", g.data.Bank.AccountNumber},
		{"Branch & IFS Code", fmt.Sprintf("%s & %s", g.data.Bank.Branch, g.data.Bank.IFSCCode)},
	}

	for _, detail := range details {
		g.pdf.SetXY(12, bankDetailsY)
		g.pdf.SetFont("Helvetica", "", 7)
		g.pdf.Cell(25, 4, detail.label)
		g.pdf.Cell(5, 4, ":")
		g.pdf.SetFont("Helvetica", "B", 7)
		g.pdf.Cell(60, 4, detail.value)
		bankDetailsY += 4
	}

	// --- Right Section: Signature ---
	g.pdf.Rect(105, y, 95, 35, "D")
	g.pdf.SetFont("Helvetica", "", 8)
	g.pdf.SetXY(105, y+2)
	g.pdf.CellFormat(95, 4, "for "+g.data.Company.Name, "", 1, "R", false, 0, "")

	g.pdf.SetFont("Helvetica", "", 7)
	g.pdf.SetXY(107, y+24) // Shifted up slightly to fit
	g.pdf.Cell(90, 4, "Prepared by")

	g.pdf.SetFont("Helvetica", "B", 8)
	g.pdf.SetXY(105, y+28)
	g.pdf.CellFormat(95, 4, "Authorised Signatory", "", 0, "R", false, 0, "")

	// --- Declaration (Full Width below Bank/Sig) ---
	y += 35
	g.pdf.Rect(10, y, 190, 15, "D")
	g.pdf.SetFont("Helvetica", "I", 7)
	g.pdf.SetXY(12, y+2)
	g.pdf.MultiCell(185, 3, "Declaration: We declare that this invoice shows the actual price of the goods described and that all particulars are true and correct.", "", "L", false)
}

// Global Wrapper Function
func GenerateTallyInvoicePDF(data InvoicePDFData, copyType string) ([]byte, error) {
	generator := NewTallyInvoiceGenerator(data, copyType)
	return generator.Generate()
}
