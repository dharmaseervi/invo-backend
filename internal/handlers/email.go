package handlers

import (
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
		ToEmail  string `json:"to_email" binding:"required"`
		ToName   string `json:"to_name" binding:"required"`
		Reminder bool   `json:"reminder"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userID := c.GetInt("user_id")
	owned, err := invoiceBelongsToUser(h.db, invoiceID, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to verify invoice"})
		return
	}
	if !owned {
		c.JSON(http.StatusForbidden, gin.H{"error": "Unauthorized"})
		return
	}

	data, err := services.FetchInvoicePDFData(h.db, invoiceID)
	if err != nil {
		log.Println("FETCH INVOICE ERROR:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch invoice"})
		return
	}

	pdfBytes, err := pdf.GenerateTallyInvoicePDF(data, "original")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate PDF"})
		return
	}

	if req.Reminder {
		err = h.emailService.SendPaymentReminderEmail(
			req.ToEmail,
			req.ToName,
			data.Invoice.InvoiceNumber,
			data.Invoice.DueDate,
			data.Invoice.AmountDue,
			pdfBytes,
		)
	} else {
		err = h.emailService.SendInvoiceEmail(
			req.ToEmail,
			req.ToName,
			data.Invoice.InvoiceNumber,
			pdfBytes,
		)
	}
	if err != nil {
		log.Println("EMAIL ERROR:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to send email"})
		return
	}

	message := "Invoice sent successfully to " + req.ToEmail
	if req.Reminder {
		message = "Reminder sent to " + req.ToEmail
	}
	c.JSON(http.StatusOK, gin.H{"message": message})
}
