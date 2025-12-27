//
// internal/pdf/generator.go
// PDF generation using gofpdf - CORRECTED API usage
//

package pdf

import (
	"bytes"
	"fmt"

	"github.com/jung-kurt/gofpdf"
)

// GenerateInvoicePDFBinary generates PDF and returns as byte slice
func GenerateInvoicePDFBinary(data InvoicePDFData) ([]byte, error) {
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.SetAutoPageBreak(true, 15)
	pdf.AddPage()

	// ========== HEADER ==========
	pdf.SetFont("Helvetica", "B", 24)
	pdf.SetTextColor(0, 0, 0)
	pdf.Cell(0, 12, "INVOICE")
	pdf.Ln(-1)

	// Company details
	pdf.SetFont("Helvetica", "", 10)
	pdf.SetTextColor(80, 80, 80)
	pdf.Cell(0, 5, data.Company.Name)
	pdf.Ln(-1)
	pdf.Cell(0, 5, data.CompanyAddress.Line1)
	pdf.Ln(-1)
	pdf.Cell(0, 5, fmt.Sprintf("%s, %s", data.CompanyAddress.City, data.CompanyAddress.State))
	pdf.Ln(-1)
	pdf.Cell(0, 5, data.CompanyAddress.Country)
	pdf.Ln(-1)

	// Invoice meta
	pdf.SetFont("Helvetica", "", 9)
	pdf.SetTextColor(120, 120, 120)
	pdf.Ln(3)
	pdf.Cell(0, 4, fmt.Sprintf("Invoice #%s", data.Invoice.InvoiceNumber))
	pdf.Ln(-1)
	pdf.Cell(0, 4, fmt.Sprintf("Date: %s | Due: %s", data.Invoice.InvoiceDate, data.Invoice.DueDate))
	pdf.Ln(-1)

	// Divider
	pdf.SetDrawColor(200, 200, 200)
	pdf.SetLineWidth(0.5)
	pdf.Ln(2)
	y := pdf.GetY()
	pdf.Line(10, y, 200, y)
	pdf.Ln(5)

	// ========== ADDRESSES ==========
	pdf.SetFont("Helvetica", "B", 10)
	pdf.SetTextColor(0, 0, 0)
	pdf.Cell(95, 6, "BILL TO")
	if data.ClientShipping != nil {
		pdf.Cell(95, 6, "SHIP TO")
	}
	pdf.Ln(-1)

	pdf.SetFont("Helvetica", "", 9)
	pdf.SetTextColor(50, 50, 50)

	// Get starting Y position
	startY := pdf.GetY()

	// Billing Address
	billingX := 10.0
	pdf.SetXY(billingX, startY)
	pdf.Cell(90, 4, data.ClientBilling.Name)
	pdf.Ln(-1)
	pdf.SetX(billingX)
	pdf.Cell(90, 4, data.ClientBilling.Line1)
	pdf.Ln(-1)
	pdf.SetX(billingX)
	pdf.Cell(90, 4, fmt.Sprintf("%s, %s", data.ClientBilling.City, data.ClientBilling.State))
	pdf.Ln(-1)
	pdf.SetX(billingX)
	pdf.Cell(90, 4, data.ClientBilling.Country)
	pdf.Ln(-1)

	billingEndY := pdf.GetY()

	// Shipping Address (if exists)
	if data.ClientShipping != nil {
		shippingX := 110.0
		pdf.SetXY(shippingX, startY)
		pdf.Cell(90, 4, data.ClientShipping.Name)
		pdf.Ln(-1)
		pdf.SetX(shippingX)
		pdf.Cell(90, 4, data.ClientShipping.Line1)
		pdf.Ln(-1)
		pdf.SetX(shippingX)
		pdf.Cell(90, 4, fmt.Sprintf("%s, %s", data.ClientShipping.City, data.ClientShipping.State))
		pdf.Ln(-1)
		pdf.SetX(shippingX)
		pdf.Cell(90, 4, data.ClientShipping.Country)
		pdf.Ln(-1)

		shippingEndY := pdf.GetY()

		// Move to the greater Y
		if shippingEndY > billingEndY {
			pdf.SetY(shippingEndY)
		} else {
			pdf.SetY(billingEndY)
		}
	} else {
		pdf.SetY(billingEndY)
	}

	pdf.Ln(5)

	// ========== ITEMS TABLE ==========
	pdf.SetFont("Helvetica", "B", 10)
	pdf.SetTextColor(0, 0, 0)
	pdf.SetFillColor(240, 240, 240)

	// Table headers - use CellFormat for borders and fill
	pdf.CellFormat(80, 8, "Description", "1", 0, "L", true, 0, "")
	pdf.CellFormat(25, 8, "Qty", "1", 0, "C", true, 0, "")
	pdf.CellFormat(30, 8, "Rate", "1", 0, "R", true, 0, "")
	pdf.CellFormat(35, 8, "Amount", "1", 1, "R", true, 0, "")

	// Table rows
	pdf.SetFont("Helvetica", "", 9)
	pdf.SetTextColor(50, 50, 50)
	pdf.SetFillColor(255, 255, 255)

	for _, item := range data.Items {
		pdf.CellFormat(80, 7, item.Name, "1", 0, "L", false, 0, "")
		pdf.CellFormat(25, 7, fmt.Sprintf("%d", item.Qty), "1", 0, "C", false, 0, "")
		pdf.CellFormat(30, 7, fmt.Sprintf("₹%.2f", item.Rate), "1", 0, "R", false, 0, "")
		pdf.CellFormat(35, 7, fmt.Sprintf("₹%.2f", item.Total), "1", 1, "R", false, 0, "")
	}

	pdf.Ln(5)

	// ========== TOTALS ==========
	pdf.SetFont("Helvetica", "", 10)
	pdf.SetTextColor(50, 50, 50)

	// Subtotal
	x := 120.0
	y = pdf.GetY()
	pdf.SetXY(x, y)
	pdf.Cell(40, 6, "Subtotal:")
	pdf.Cell(35, 6, fmt.Sprintf("₹%.2f", data.Invoice.Subtotal))
	pdf.Ln(-1)

	// Tax
	pdf.SetX(x)
	pdf.Cell(40, 6, "Tax (18%):")
	pdf.Cell(35, 6, fmt.Sprintf("₹%.2f", data.Invoice.Tax))
	pdf.Ln(-1)

	// Divider
	pdf.SetDrawColor(200, 200, 200)
	y = pdf.GetY()
	pdf.Line(x, y+1, x+75, y+1)
	pdf.Ln(3)

	// Total
	pdf.SetFont("Helvetica", "B", 12)
	pdf.SetTextColor(0, 0, 0)
	pdf.SetX(x)
	pdf.Cell(40, 8, "TOTAL:")
	pdf.Cell(35, 8, fmt.Sprintf("₹%.2f", data.Invoice.Total))
	pdf.Ln(-1)

	// ========== FOOTER ==========
	pdf.Ln(8)
	pdf.SetFont("Helvetica", "I", 9)
	pdf.SetTextColor(120, 120, 120)
	if data.Invoice.Notes != "" {
		pdf.MultiCell(0, 5, fmt.Sprintf("Notes: %s", data.Invoice.Notes), "", "L", false)
	}

	pdf.Ln(3)
	pdf.Cell(0, 4, "Thank you for your business!")

	// Convert to bytes
	var buf bytes.Buffer
	err := pdf.Output(&buf)
	if err != nil {
		return nil, fmt.Errorf("failed to output PDF: %w", err)
	}

	return buf.Bytes(), nil
}
