package handlers

import (
	"fmt"
	"invo-server/internal/pdf"
	"invo-server/internal/services"
	"log"
	"net/http"
	"strconv"

	"database/sql"

	"github.com/gin-gonic/gin"
)

type EmailHandler struct {
	emailService *services.EmailService
	db           *sql.DB
}

func NewEmailHandler(emailService *services.EmailService, db *sql.DB) *EmailHandler {
	return &EmailHandler{
		emailService: emailService,
		db:           db,
	}
}
func (h *EmailHandler) SendInvoiceEmail(c *gin.Context) {
	invoiceIDStr := c.Param("id")
	invoiceID, err := strconv.Atoi(invoiceIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid invoice ID"})
		return
	}

	var req struct {
		ToEmail string `json:"to_email" binding:"required"`
		ToName  string `json:"to_name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	data, err := services.FetchInvoicePDFData(h.db, invoiceID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to fetch invoice: %v", err)})
		return
	}

	pdfBytes, err := pdf.GenerateTallyInvoicePDF(data, "original")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate PDF"})
		return
	}

	err = h.emailService.SendInvoiceEmail(
		req.ToEmail,
		req.ToName,
		data.Invoice.InvoiceNumber,
		pdfBytes,
	)
	if err != nil {
		log.Println("EMAIL ERROR:", err) // add this
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to send email: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Invoice sent successfully to " + req.ToEmail})
}
