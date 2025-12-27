//
// handlers/invoice_pdf_handler.go
// Refactored to return binary PDF directly
//

package handlers

import (
	"fmt"
	"log"
	"net/http"
	"strconv"

	database "invo-server/internal/db"
	"invo-server/internal/pdf"
	"invo-server/internal/services"

	"github.com/gin-gonic/gin"
)

type InvoicePDFHandler struct {
	db *database.Database
}

func NewInvoicePDFHandler(db *database.Database) *InvoicePDFHandler {
	return &InvoicePDFHandler{db: db}
}

// ============================================
// GET /api/v1/invoices/:id/pdf
// Return PDF as binary data (no disk storage)
// ============================================
func (h *InvoicePDFHandler) GetInvoicePDF(c *gin.Context) {
	invoiceID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid invoice id"})
		return
	}

	userID := c.GetInt("user_id")

	log.Printf("📄 Generating PDF for Invoice ID: %d, User ID: %d", invoiceID, userID)

	// 🔐 Authorization - verify user owns this invoice
	var authorized bool
	err = h.db.DB.QueryRow(`
		SELECT EXISTS (
			SELECT 1
			FROM invoices i
			JOIN companies c ON c.id = i.company_id
			WHERE i.id = $1 AND c.user_id = $2
		)
	`, invoiceID, userID).Scan(&authorized)

	if err != nil || !authorized {
		log.Printf("❌ Unauthorized access to invoice %d", invoiceID)
		c.JSON(http.StatusForbidden, gin.H{"error": "Unauthorized"})
		return
	}

	// 📊 Fetch invoice data
	pdfData, err := services.FetchInvoicePDFData(h.db.DB, invoiceID)
	if err != nil {
		log.Printf("❌ Failed to fetch invoice data: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch invoice data"})
		return
	}

	// 📄 Generate PDF in memory
	pdfBytes, err := pdf.GenerateInvoicePDFBinary(pdfData)
	if err != nil {
		log.Printf("❌ PDF generation failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate PDF"})
		return
	}

	fileName := fmt.Sprintf("Invoice_%s.pdf", pdfData.Invoice.InvoiceNumber)

	// 📤 Return PDF as binary data
	c.Header("Content-Type", "application/pdf")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", fileName))
	c.Header("Content-Length", fmt.Sprintf("%d", len(pdfBytes)))

	// Optional: Cache headers
	c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
	c.Header("Pragma", "no-cache")

	log.Printf("✅ PDF generated successfully: %s (%d bytes)", fileName, len(pdfBytes))

	// Send binary data directly
	c.Data(http.StatusOK, "application/pdf", pdfBytes)
}
