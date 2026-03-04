package handlers

import (
	"database/sql"
	"fmt"
	"invo-server/internal/models"
	"invo-server/internal/services"
	"log"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

type CreditNoteHandler struct {
	service *services.CreditNoteService
	db      *sql.DB
}

func NewCreditNoteHandler(service *services.CreditNoteService, db *sql.DB) *CreditNoteHandler {
	return &CreditNoteHandler{service: service, db: db}
}

func (h *CreditNoteHandler) Create(c *gin.Context) {
	var req models.CreditNoteRequestDTO
	userID := c.GetInt("user_id")

	if err := c.ShouldBindJSON(&req); err != nil {
		fmt.Println("Error binding JSON:", err)
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	// 5️⃣ Begin transaction
	tx, err := h.db.Begin()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to start transaction"})
		return
	}

	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	var companyID int64

	err = tx.QueryRow(`
		SELECT id FROM companies WHERE user_id = $1
	`, userID).Scan(&companyID)

	if err != nil {
		c.JSON(403, gin.H{"error": "unauthorized"})
		return
	}

	if err := h.service.CreateTx(tx, companyID, req); err != nil {
		fmt.Println("Error creating credit note:", err)
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	// 1️⃣3️⃣ Commit transaction
	if err = tx.Commit(); err != nil {
		log.Printf("Failed to commit transaction: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to commit transaction",
		})
		return
	}

	committed = true

	c.JSON(201, gin.H{"message": "Credit note created"})
}

func (h *CreditNoteHandler) GetAll(c *gin.Context) {
	userID := c.GetInt("user_id")

	var companyID int64
	err := h.db.QueryRow(`
		SELECT id FROM companies WHERE user_id = $1
	`, userID).Scan(&companyID)

	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "unauthorized"})
		return
	}

	result, err := h.service.GetAll(companyID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}

func (h *CreditNoteHandler) GetByID(c *gin.Context) {
	userID := c.GetInt("user_id")

	cnID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(400, gin.H{"error": "invalid credit note id"})
		return
	}

	tx, err := h.db.Begin()
	if err != nil {
		c.JSON(500, gin.H{"error": "failed to start transaction"})
		return
	}
	defer tx.Rollback()

	var companyID int64
	err = tx.QueryRow(`SELECT id FROM companies WHERE user_id = $1`, userID).
		Scan(&companyID)
	if err != nil {
		c.JSON(403, gin.H{"error": "unauthorized"})
		return
	}

	result, err := h.service.GetByID(tx, companyID, cnID)
	if err != nil {
		c.JSON(404, gin.H{"error": "credit note not found"})
		return
	}

	if err := tx.Commit(); err != nil {
		c.JSON(500, gin.H{"error": "failed to commit"})
		return
	}
	fmt.Println("Credit Note fetched:", result)
	// ✅ EXACT SHAPE REQUIRED BY iOS
	c.JSON(http.StatusOK, result)
}
