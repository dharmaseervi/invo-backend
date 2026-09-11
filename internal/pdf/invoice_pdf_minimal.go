package pdf

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/jung-kurt/gofpdf"
)

// MinimalInvoiceGenerator — spare, whitespace-first layout: no fills or boxes,
// thin hairline rules only, right-aligned totals. Closest to a Stripe-style invoice.
type MinimalInvoiceGenerator struct {
	pdf      *gofpdf.Fpdf
	data     InvoicePDFData
	copyType string
}

func NewMinimalInvoiceGenerator(data InvoicePDFData, copyType string) *MinimalInvoiceGenerator {
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(0, 0, 0)
	pdf.SetAutoPageBreak(true, 15)
	return &MinimalInvoiceGenerator{pdf: pdf, data: data, copyType: strings.ToUpper(copyType)}
}

func (g *MinimalInvoiceGenerator) Generate() ([]byte, error) {
	pdf := g.pdf

	pdf.SetFooterFunc(func() {
		pdf.SetY(-10)
		pdf.SetFont("Helvetica", "", 7)
		pdf.SetTextColor(170, 170, 170)
		pdf.CellFormat(pageW, 5, fmt.Sprintf("%d", pdf.PageNo()), "", 0, "C", false, 0, "")
		pdf.SetTextColor(0, 0, 0)
	})

	pdf.AddPage()

	y := g.drawHeader(marginT + 4)
	y = g.drawParties(y)
	y = g.drawItemsTable(y)
	y = g.drawTotalsSection(y)
	g.drawFooter(y)

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (g *MinimalInvoiceGenerator) drawHeader(y float64) float64 {
	pdf := g.pdf

	pdf.SetFont("Helvetica", "", 26)
	pdf.SetXY(marginL, y)
	pdf.Cell(pageW/2, 10, g.data.Invoice.HeaderTitle("Invoice"))

	pdf.SetFont("Helvetica", "", 8)
	pdf.SetTextColor(150, 150, 150)
	pdf.SetXY(marginL+pageW/2, y+3)
	pdf.CellFormat(pageW/2, 5, g.copyType+" COPY", "", 1, "R", false, 0, "")
	pdf.SetXY(marginL+pageW/2, y+8)
	pdf.CellFormat(pageW/2, 5, g.data.Invoice.InvoiceNumber, "", 0, "R", false, 0, "")
	pdf.SetTextColor(0, 0, 0)

	return y + 20
}

func (g *MinimalInvoiceGenerator) drawParties(y float64) float64 {
	pdf := g.pdf
	mid := marginL + pageW/2 + 10

	// FROM
	pdf.SetFont("Helvetica", "", 7.5)
	pdf.SetTextColor(150, 150, 150)
	pdf.SetXY(marginL, y)
	pdf.Cell(60, 4, "From")

	pdf.SetFont("Helvetica", "B", 9)
	pdf.SetTextColor(0, 0, 0)
	pdf.SetXY(marginL, y+5)
	pdf.Cell(pageW/2-10, 4, g.data.Company.Name)

	pdf.SetFont("Helvetica", "", 8)
	pdf.SetTextColor(120, 120, 120)
	pdf.SetXY(marginL, y+10)
	pdf.MultiCell(pageW/2-14, 4,
		fmt.Sprintf("%s\n%s, %s - %s",
			g.data.CompanyAddress.Line1, g.data.CompanyAddress.City,
			g.data.CompanyAddress.State, g.data.CompanyAddress.Zip),
		"", "L", false)
	pdf.SetTextColor(0, 0, 0)

	// BILL TO
	pdf.SetFont("Helvetica", "", 7.5)
	pdf.SetTextColor(150, 150, 150)
	pdf.SetXY(mid, y)
	pdf.Cell(60, 4, "Bill to")

	pdf.SetFont("Helvetica", "B", 9)
	pdf.SetTextColor(0, 0, 0)
	pdf.SetXY(mid, y+5)
	pdf.Cell(pageW/2-10, 4, g.data.ClientBilling.Name)

	pdf.SetFont("Helvetica", "", 8)
	pdf.SetTextColor(120, 120, 120)
	pdf.SetXY(mid, y+10)
	pdf.MultiCell(pageW/2-14, 4,
		fmt.Sprintf("%s, %s, %s - %s",
			g.data.ClientBilling.Line1, g.data.ClientBilling.City,
			g.data.ClientBilling.State, g.data.ClientBilling.Zip),
		"", "L", false)
	pdf.SetTextColor(0, 0, 0)

	y += 26

	// Dates row
	pdf.SetFont("Helvetica", "", 7.5)
	pdf.SetTextColor(150, 150, 150)
	pdf.SetXY(marginL, y)
	pdf.Cell(40, 4, "Invoice date")
	pdf.SetXY(marginL+45, y)
	pdf.Cell(40, 4, "Due date")
	if g.data.ClientShipping != nil {
		pdf.SetXY(mid, y)
		pdf.Cell(60, 4, "Ship to")
	}

	pdf.SetFont("Helvetica", "B", 8.5)
	pdf.SetTextColor(0, 0, 0)
	pdf.SetXY(marginL, y+5)
	pdf.Cell(40, 4, g.data.Invoice.InvoiceDate)
	pdf.SetXY(marginL+45, y+5)
	pdf.Cell(40, 4, g.data.Invoice.DueDate)

	if g.data.ClientShipping != nil {
		pdf.SetFont("Helvetica", "", 8)
		pdf.SetTextColor(120, 120, 120)
		pdf.SetXY(mid, y+5)
		pdf.MultiCell(pageW/2-14, 4,
			fmt.Sprintf("%s\n%s, %s",
				g.data.ClientShipping.Line1, g.data.ClientShipping.City, g.data.ClientShipping.State),
			"", "L", false)
		pdf.SetTextColor(0, 0, 0)
	}

	y += 18
	pdf.SetDrawColor(220, 220, 220)
	pdf.SetLineWidth(0.2)
	pdf.Line(marginL, y, marginL+pageW, y)
	pdf.SetDrawColor(0, 0, 0)

	return y + 8
}

func (g *MinimalInvoiceGenerator) drawItemsTable(y float64) float64 {
	pdf := g.pdf

	wDesc, wQty, wRate, wTax, wAmt := 90.0, 20.0, 30.0, 20.0, 30.0
	rowH := 8.0

	drawHeader := func(startY float64) {
		pdf.SetFont("Helvetica", "", 7.5)
		pdf.SetTextColor(150, 150, 150)
		pdf.SetXY(marginL, startY)
		pdf.CellFormat(wDesc, 5, "Description", "", 0, "L", false, 0, "")
		pdf.CellFormat(wQty, 5, "Qty", "", 0, "C", false, 0, "")
		pdf.CellFormat(wRate, 5, "Rate", "", 0, "R", false, 0, "")
		pdf.CellFormat(wTax, 5, "Tax", "", 0, "C", false, 0, "")
		pdf.CellFormat(wAmt, 5, "Amount", "", 1, "R", false, 0, "")
		pdf.SetDrawColor(220, 220, 220)
		pdf.Line(marginL, startY+6, marginL+pageW, startY+6)
		pdf.SetDrawColor(0, 0, 0)
		pdf.SetTextColor(0, 0, 0)
	}

	drawHeader(y)
	y += 9

	for i, item := range g.data.Items {
		if y > 250 {
			pdf.AddPage()
			y = marginT + 10
			drawHeader(y)
			y += 9
		}

		pdf.SetFont("Helvetica", "", 8.5)
		pdf.SetXY(marginL, y)
		pdf.CellFormat(wDesc, rowH, item.Name, "", 0, "L", false, 0, "")
		pdf.CellFormat(wQty, rowH, fmt.Sprintf("%d", item.Qty), "", 0, "C", false, 0, "")
		pdf.CellFormat(wRate, rowH, fmt.Sprintf("%.2f", item.Rate), "", 0, "R", false, 0, "")
		pdf.CellFormat(wTax, rowH, fmt.Sprintf("%.1f%%", item.TaxRate), "", 0, "C", false, 0, "")
		pdf.CellFormat(wAmt, rowH, fmt.Sprintf("%.2f", item.Taxable), "", 1, "R", false, 0, "")

		if i != len(g.data.Items)-1 {
			pdf.SetDrawColor(240, 240, 240)
			pdf.Line(marginL, y+rowH-1, marginL+pageW, y+rowH-1)
			pdf.SetDrawColor(0, 0, 0)
		}
		y += rowH
	}

	pdf.SetDrawColor(220, 220, 220)
	pdf.Line(marginL, y, marginL+pageW, y)
	pdf.SetDrawColor(0, 0, 0)

	return y + 8
}

func (g *MinimalInvoiceGenerator) drawTotalsSection(y float64) float64 {
	pdf := g.pdf
	inv := g.data.Invoice
	taxRows := inv.TaxSummaryRows()

	rowCount := len(taxRows) + 1
	if inv.Discount > 0 {
		rowCount++
	}

	if y+float64(rowCount)*6+24 > 258 {
		pdf.AddPage()
		y = marginT + 10
	}

	x := marginL + pageW - 75

	rowY := y
	g.totalRow(x, rowY, "Subtotal", inv.Subtotal, false)
	rowY += 6
	for _, row := range taxRows {
		g.totalRow(x, rowY, row.Label, row.Amount, false)
		rowY += 6
	}
	if inv.Discount > 0 {
		g.totalRow(x, rowY, "Discount", -inv.Discount, false)
		rowY += 6
	}

	y = rowY
	pdf.SetDrawColor(0, 0, 0)
	pdf.SetLineWidth(0.3)
	pdf.Line(x, y, x+75, y)
	pdf.SetLineWidth(0.2)
	y += 3

	g.totalRow(x, y, "Total", g.data.Invoice.Total, true)

	y2 := y + 12
	pdf.SetFont("Helvetica", "I", 7.5)
	pdf.SetTextColor(140, 140, 140)
	pdf.SetXY(marginL, y2)
	pdf.Cell(pageW, 5, "In words: "+AmountToWords(g.data.Invoice.Total))
	pdf.SetTextColor(0, 0, 0)

	return y2 + 10
}

func (g *MinimalInvoiceGenerator) drawFooter(y float64) float64 {
	pdf := g.pdf
	mid := marginL + pageW/2 + 10

	if y > 235 {
		pdf.AddPage()
		y = marginT + 10
	}

	pdf.SetDrawColor(220, 220, 220)
	pdf.Line(marginL, y, marginL+pageW, y)
	pdf.SetDrawColor(0, 0, 0)
	y += 8

	pdf.SetFont("Helvetica", "", 7.5)
	pdf.SetTextColor(150, 150, 150)
	pdf.SetXY(marginL, y)
	pdf.Cell(60, 4, "Bank details")

	g.plainRow(y+5, "Bank Name", g.data.Bank.BankName)
	g.plainRow(y+10, "Account No.", g.data.Bank.AccountNumber)
	g.plainRow(y+15, "IFSC Code", g.data.Bank.IFSCCode)
	g.plainRow(y+20, "Branch", g.data.Bank.Branch)

	pdf.SetDrawColor(150, 150, 150)
	pdf.SetLineWidth(0.2)
	pdf.Line(mid, y+22, marginL+pageW, y+22)
	pdf.SetDrawColor(0, 0, 0)
	pdf.SetFont("Helvetica", "", 7)
	pdf.SetTextColor(140, 140, 140)
	pdf.SetXY(mid, y+23)
	pdf.CellFormat(pageW/2-10, 4, "Authorised Signatory - "+g.data.Company.Name, "", 0, "L", false, 0, "")
	pdf.SetTextColor(0, 0, 0)

	y2 := y + 30
	pdf.SetFont("Helvetica", "I", 6.5)
	pdf.SetTextColor(160, 160, 160)
	pdf.SetXY(marginL, y2)
	pdf.MultiCell(pageW, 3.5,
		"Declaration: We declare that this invoice shows the actual price of the goods "+
			"described and that all particulars are true and correct. Goods once sold will not be taken back.",
		"", "L", false)
	pdf.SetTextColor(0, 0, 0)

	return y2 + 8
}

func (g *MinimalInvoiceGenerator) totalRow(x, y float64, label string, value float64, bold bool) {
	pdf := g.pdf
	if bold {
		pdf.SetFont("Helvetica", "B", 10)
	} else {
		pdf.SetFont("Helvetica", "", 8.5)
		pdf.SetTextColor(120, 120, 120)
	}
	pdf.SetXY(x, y)
	pdf.Cell(35, 5, label)
	if bold {
		pdf.SetFont("Helvetica", "B", 10)
	} else {
		pdf.SetFont("Helvetica", "", 8.5)
	}
	pdf.SetTextColor(0, 0, 0)
	pdf.SetXY(x+35, y)
	pdf.CellFormat(40, 5, fmt.Sprintf("INR %.2f", value), "", 0, "R", false, 0, "")
}

func (g *MinimalInvoiceGenerator) plainRow(y float64, label, value string) {
	pdf := g.pdf
	pdf.SetFont("Helvetica", "", 7.5)
	pdf.SetTextColor(150, 150, 150)
	pdf.SetXY(marginL, y)
	pdf.Cell(24, 4, label)
	pdf.SetFont("Helvetica", "B", 7.5)
	pdf.SetTextColor(0, 0, 0)
	pdf.SetXY(marginL+26, y)
	pdf.Cell(55, 4, value)
}
