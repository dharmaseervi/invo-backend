package handlers

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
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

// POST /api/v1/invoices/:id/generate-pdf
func (h *InvoicePDFHandler) GenerateInvoicePDF(c *gin.Context) {
	invoiceID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid invoice id"})
		return
	}

	userID := c.GetInt("user_id")

	// 🔐 Authorization
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
		c.JSON(http.StatusForbidden, gin.H{"error": "Unauthorized"})
		return
	}

	// 📄 Fetch data
	pdfData, err := services.FetchInvoicePDFData(h.db.DB, invoiceID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 📁 Paths
	fileName := fmt.Sprintf("%s.pdf", pdfData.Invoice.InvoiceNumber)
	fsPath := filepath.Join("storage", "invoices", fileName)
	publicURL := fmt.Sprintf(
		"%s/storage/invoices/%s",
		os.Getenv("BASE_URL"), // e.g. http://localhost:8080
		fileName,
	)

	// 📄 Generate PDF
	err = pdf.GenerateInvoicePDF(
		"internal/pdf/templates/invoice.html",
		pdfData,
		fsPath,
	)
	if err != nil {
		log.Printf("PDF ERROR: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 💾 Persist public URL
	_, err = h.db.DB.Exec(`
		UPDATE invoices
		SET pdf_url = $1,
		    pdf_generated_at = NOW()
		WHERE id = $2
	`, publicURL, invoiceID)

	if err != nil {
		log.Printf("DB ERROR: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update invoice"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Invoice PDF generated successfully",
		"pdf_url": publicURL,
	})
}
