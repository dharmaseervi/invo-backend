package handlers

import (
	"fmt"
	"net/http"

	database "invo-server/internal/db"
	"invo-server/internal/models"
	"invo-server/internal/services"

	"github.com/gin-gonic/gin"
)

type PaymentHandler struct {
	db      *database.Database
	service *services.PaymentService
}

func NewPaymentHandler(db *database.Database, service *services.PaymentService) *PaymentHandler {
	return &PaymentHandler{
		db:      db,
		service: service,
	}
}

// POST /api/v1/payments
func (h *PaymentHandler) RecordPayment(c *gin.Context) {
	var req models.PaymentRequestDTO
	userID := c.GetInt("user_id")

	if err := c.ShouldBindJSON(&req); err != nil {
		fmt.Println("SQL ERROR:", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	tx, err := h.db.DB.Begin()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tx failed"})
		return
	}
	defer tx.Rollback()

	// auth
	var companyID int64
	err = tx.QueryRow(`
		SELECT c.id
		FROM clients cl
		JOIN companies c ON c.id = cl.company_id
		WHERE cl.id = $1 AND c.user_id = $2
	`, req.ClientID, userID).Scan(&companyID)

	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "unauthorized"})
		return
	}

	err = h.service.RecordPaymentTx(tx, companyID, req.ClientID, req)
	if err != nil {
		fmt.Println("SQL ERROR:", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	tx.Commit()

	c.JSON(http.StatusCreated, gin.H{
		"message": "Payment recorded successfully",
	})
}
