package pdf

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/jung-kurt/gofpdf"
)

// DebitNoteData carries everything needed to render a debit note PDF.
type DebitNoteData struct {
	Company      Company
	CompanyGSTIN string
	CompanyAddr  Address
	Supplier     DebitNoteParty
	Return       DebitNoteReturn
	Items        []DebitNoteItem
}

type DebitNoteParty struct {
	Name   string
	GSTIN  string
	Phone  string
	Line1  string
	City   string
	State  string
	Zip    string
}

type DebitNoteReturn struct {
	ReturnNumber string
	ReturnDate   string
	BillNumber   string // empty if not linked
	Subtotal     float64
	Tax          float64
	Total        float64
	Reason       string
}

type DebitNoteItem struct {
	Name    string
	Qty     int
	Rate    float64
	TaxRate float64
	Total   float64
}

// GenerateDebitNotePDF renders a debit note as a PDF and returns the bytes.
func GenerateDebitNotePDF(d DebitNoteData) ([]byte, error) {
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(0, 0, 0)
	pdf.SetAutoPageBreak(true, 15)

	pdf.SetFooterFunc(func() {
		pdf.SetY(-10)
		pdf.SetFont("Helvetica", "I", 7)
		pdf.SetTextColor(150, 150, 150)
		pdf.CellFormat(pageW, 5,
			fmt.Sprintf("Page %d - %s", pdf.PageNo(), d.Company.Name),
			"", 0, "C", false, 0, "")
		pdf.SetTextColor(0, 0, 0)
	})

	pdf.AddPage()
	y := marginT

	y = dnDrawHeader(pdf, d, y)
	y = dnDrawDetails(pdf, d, y)
	y = dnDrawSupplier(pdf, d, y)
	y = dnDrawItems(pdf, d, y)
	y = dnDrawTotals(pdf, d, y)
	dnDrawReason(pdf, d, y)

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func dnDrawHeader(pdf *gofpdf.Fpdf, d DebitNoteData, y float64) float64 {
	// Title bar
	pdf.SetFillColor(20, 20, 20)
	pdf.Rect(marginL, y, pageW, 10, "F")
	pdf.SetFont("Helvetica", "B", 12)
	pdf.SetTextColor(255, 255, 255)
	pdf.SetXY(marginL, y)
	pdf.CellFormat(pageW, 10, "DEBIT NOTE", "", 0, "C", false, 0, "")
	pdf.SetTextColor(0, 0, 0)
	return y + 10
}

func dnDrawDetails(pdf *gofpdf.Fpdf, d DebitNoteData, y float64) float64 {
	h := 34.0
	mid := marginL + pageW/2

	// Company name
	pdf.SetFont("Helvetica", "B", 10)
	pdf.SetXY(marginL+2, y+3)
	pdf.Cell(pageW/2-4, 5, d.Company.Name)

	// Company address
	pdf.SetFont("Helvetica", "", 8)
	pdf.SetTextColor(60, 60, 60)
	pdf.SetXY(marginL+2, y+9)
	pdf.MultiCell(pageW/2-4, 4,
		fmt.Sprintf("%s\n%s, %s - %s",
			d.CompanyAddr.Line1, d.CompanyAddr.City,
			d.CompanyAddr.State, d.CompanyAddr.Zip),
		"", "L", false)

	if d.Company.Phone != "" {
		pdf.SetFont("Helvetica", "", 7.5)
		pdf.SetXY(marginL+2, y+22)
		pdf.Cell(pageW/2-4, 4, "Ph: "+d.Company.Phone)
	}
	if d.CompanyGSTIN != "" {
		pdf.SetFont("Helvetica", "", 7.5)
		pdf.SetXY(marginL+2, y+27)
		pdf.Cell(pageW/2-4, 4, "GSTIN: "+d.CompanyGSTIN)
	}
	pdf.SetTextColor(0, 0, 0)

	// Vertical divider
	pdf.SetDrawColor(200, 200, 200)
	pdf.Line(mid, y, mid, y+h)
	pdf.SetDrawColor(0, 0, 0)

	// Return details on the right
	dnLabelValue(pdf, mid+3, y+4, "Debit Note No.", d.Return.ReturnNumber)
	dnLabelValue(pdf, mid+3, y+11, "Date", d.Return.ReturnDate)
	if d.Return.BillNumber != "" {
		dnLabelValue(pdf, mid+3, y+18, "Against Bill", d.Return.BillNumber)
	}

	// Bottom border
	pdf.SetDrawColor(0, 0, 0)
	pdf.Line(marginL, y+h, marginL+pageW, y+h)
	return y + h
}

func dnDrawSupplier(pdf *gofpdf.Fpdf, d DebitNoteData, y float64) float64 {
	h := 24.0

	// Section label
	pdf.SetFillColor(245, 245, 245)
	pdf.Rect(marginL, y, pageW, 6, "F")
	pdf.SetFont("Helvetica", "B", 8)
	pdf.SetXY(marginL+2, y+1)
	pdf.Cell(pageW, 5, "RETURN TO SUPPLIER")

	// Supplier details
	pdf.SetFont("Helvetica", "B", 9)
	pdf.SetXY(marginL+2, y+8)
	pdf.Cell(pageW/2, 5, d.Supplier.Name)

	pdf.SetFont("Helvetica", "", 8)
	pdf.SetTextColor(60, 60, 60)
	addr := strings.Join(filterEmpty([]string{
		d.Supplier.Line1,
		joinComma(d.Supplier.City, d.Supplier.State),
	}), ", ")
	if addr != "" {
		pdf.SetXY(marginL+2, y+14)
		pdf.Cell(pageW/2, 4, addr)
	}
	if d.Supplier.GSTIN != "" {
		pdf.SetXY(marginL+2, y+19)
		pdf.Cell(pageW/2, 4, "GSTIN: "+d.Supplier.GSTIN)
	}
	pdf.SetTextColor(0, 0, 0)

	pdf.SetDrawColor(0, 0, 0)
	pdf.Line(marginL, y+h, marginL+pageW, y+h)
	return y + h
}

func dnDrawItems(pdf *gofpdf.Fpdf, d DebitNoteData, y float64) float64 {
	// Column widths: #, Item, Qty, Rate, Tax%, Total
	cols := []float64{8, 82, 18, 28, 22, 32}
	headers := []string{"#", "Item", "Qty", "Rate", "Tax %", "Amount"}

	// Header row
	pdf.SetFillColor(20, 20, 20)
	pdf.SetTextColor(255, 255, 255)
	pdf.SetFont("Helvetica", "B", 8)
	x := marginL
	for i, w := range cols {
		pdf.SetXY(x, y)
		pdf.CellFormat(w, 6, headers[i], "", 0, "C", true, 0, "")
		x += w
	}
	pdf.Ln(-1)
	y += 6

	pdf.SetTextColor(0, 0, 0)
	pdf.SetFont("Helvetica", "", 8)

	for i, item := range d.Items {
		pdf.SetFillColor(255, 255, 255)
		if i%2 == 1 {
			pdf.SetFillColor(248, 248, 248)
		}
		x = marginL
		cells := []string{
			fmt.Sprintf("%d", i+1),
			item.Name,
			fmt.Sprintf("%d", item.Qty),
			fmt.Sprintf("%.2f", item.Rate),
			fmt.Sprintf("%.0f%%", item.TaxRate),
			fmt.Sprintf("%.2f", item.Total),
		}
		aligns := []string{"C", "L", "C", "R", "C", "R"}
		for j, w := range cols {
			pdf.SetXY(x, y)
			pdf.CellFormat(w, 6, cells[j], "", 0, aligns[j], true, 0, "")
			x += w
		}
		pdf.Ln(-1)
		y += 6
	}

	pdf.SetDrawColor(0, 0, 0)
	pdf.Line(marginL, y, marginL+pageW, y)
	return y + 2
}

func dnDrawTotals(pdf *gofpdf.Fpdf, d DebitNoteData, y float64) float64 {
	col1 := 148.0 // label column width
	col2 := 42.0  // value column width
	x := marginL + pageW - col1 - col2

	row := func(label string, amount float64, bold bool) {
		if bold {
			pdf.SetFont("Helvetica", "B", 9)
		} else {
			pdf.SetFont("Helvetica", "", 8.5)
		}
		pdf.SetXY(x, y)
		pdf.CellFormat(col1, 6, label, "", 0, "R", false, 0, "")
		pdf.CellFormat(col2, 6, fmt.Sprintf("%.2f", amount), "", 0, "R", false, 0, "")
		y += 6
	}

	row("Subtotal", d.Return.Subtotal, false)
	row("Tax", d.Return.Tax, false)

	// Total line with background
	pdf.SetFillColor(240, 240, 240)
	pdf.Rect(x, y, col1+col2, 7, "F")
	pdf.SetFont("Helvetica", "B", 10)
	pdf.SetXY(x, y)
	pdf.CellFormat(col1, 7, "Total Credit", "", 0, "R", false, 0, "")
	pdf.CellFormat(col2, 7, fmt.Sprintf("%.2f", d.Return.Total), "", 0, "R", false, 0, "")
	y += 7

	// Amount in words
	pdf.SetFont("Helvetica", "I", 8)
	pdf.SetTextColor(80, 80, 80)
	pdf.SetXY(marginL+2, y+4)
	pdf.Cell(pageW, 5, AmountToWords(d.Return.Total))
	pdf.SetTextColor(0, 0, 0)

	return y + 12
}

func dnDrawReason(pdf *gofpdf.Fpdf, d DebitNoteData, y float64) {
	if d.Return.Reason == "" {
		return
	}
	pdf.SetFont("Helvetica", "B", 8)
	pdf.SetXY(marginL+2, y+4)
	pdf.Cell(30, 5, "Reason for Return:")
	pdf.SetFont("Helvetica", "", 8)
	pdf.SetTextColor(60, 60, 60)
	pdf.SetXY(marginL+34, y+4)
	pdf.MultiCell(pageW-34, 4.5, d.Return.Reason, "", "L", false)
	pdf.SetTextColor(0, 0, 0)
}

func dnLabelValue(pdf *gofpdf.Fpdf, x, y float64, label, value string) {
	pdf.SetFont("Helvetica", "", 7.5)
	pdf.SetTextColor(120, 120, 120)
	pdf.SetXY(x, y)
	pdf.Cell(40, 4, label)
	pdf.SetFont("Helvetica", "B", 8.5)
	pdf.SetTextColor(0, 0, 0)
	pdf.SetXY(x, y+4.5)
	pdf.Cell(40, 4, value)
}

func filterEmpty(ss []string) []string {
	out := ss[:0]
	for _, s := range ss {
		if strings.TrimSpace(s) != "" {
			out = append(out, s)
		}
	}
	return out
}

func joinComma(parts ...string) string {
	return strings.Join(filterEmpty(parts), ", ")
}
