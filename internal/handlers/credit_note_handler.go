package handlers

import (
	"database/sql"
	"errors"
	"invo-server/internal/models"
	"invo-server/internal/services"
	"log"
	"net/http"
	"strconv"
	"strings"

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
		c.JSON(400, gin.H{"error": "Invalid input"})
		return
	}

	// 5️⃣ Begin transaction
	tx, err := h.db.BeginTx(c.Request.Context(), nil)
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

	var exists bool
	err = tx.QueryRowContext(c.Request.Context(),
		`
		SELECT EXISTS (SELECT 1 FROM companies WHERE id = $1 AND user_id = $2
            UNION ALL
            SELECT 1 FROM company_members WHERE company_id = $1 AND user_id = $2)
	`, req.CompanyID, userID).Scan(&exists)

	if err != nil || !exists {
		c.JSON(403, gin.H{"error": "unauthorized"})
		return
	}

	if err := h.service.CreateTx(c.Request.Context(),
		tx, req.CompanyID, req); err != nil {
		var input services.CreditNoteInputError
		if errors.As(err, &input) {
			c.JSON(400, gin.H{"error": input.Msg})
			return
		}
		log.Println("failed to create credit note:", err)
		c.JSON(400, gin.H{"error": "Failed to create credit note"})
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

	companyIDStr := c.Query("company_id")

	if companyIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "company_id is required"})
		return
	}

	companyID, err := strconv.ParseInt(companyIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid company_id"})
		return
	}

	// Verify ownership
	var exists bool
	h.db.QueryRowContext(c.Request.Context(),
		`
        SELECT EXISTS(
            SELECT 1 FROM companies WHERE id = $1 AND user_id = $2
            UNION ALL
            SELECT 1 FROM company_members WHERE company_id = $1 AND user_id = $2
        )
    `, companyID, userID).Scan(&exists)

	if !exists {
		c.JSON(http.StatusForbidden, gin.H{"error": "unauthorized"})
		return
	}

	result, err := h.service.GetAll(c.Request.Context(),
		companyID,
		strings.TrimSpace(c.Query("search")),
		strings.ToLower(strings.TrimSpace(c.Query("type"))),
		clampPageSize(mustAtoi(c.Query("limit")), 0),
		mustAtoi(c.Query("offset")),
	)
	if err != nil {
		log.Println("failed to fetch credit notes:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch credit notes"})
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

	tx, err := h.db.BeginTx(c.Request.Context(), nil)
	if err != nil {
		c.JSON(500, gin.H{"error": "failed to start transaction"})
		return
	}
	defer tx.Rollback()

	var companyID int64
	err = tx.QueryRowContext(c.Request.Context(),
		`
		SELECT cn.company_id
		FROM credit_notes cn
		JOIN companies c ON c.id = cn.company_id
		WHERE cn.id = $1 AND c.id IN (SELECT company_id FROM companies_for_user($2))
	`, cnID, userID).Scan(&companyID)
	if err != nil {
		c.JSON(404, gin.H{"error": "credit note not found"})
		return
	}

	result, err := h.service.GetByID(c.Request.Context(),
		tx, companyID, cnID)
	if err != nil {
		c.JSON(404, gin.H{"error": "credit note not found"})
		return
	}

	if err := tx.Commit(); err != nil {
		c.JSON(500, gin.H{"error": "failed to commit"})
		return
	}
	c.JSON(http.StatusOK, result)
}

// GetSummary returns the counts and amounts a credit-note screen shows above its rows.
//
// GET /api/v1/credit-notes/summary?company_id=&search=
func (h *CreditNoteHandler) GetSummary(c *gin.Context) {
	userID := c.GetInt("user_id")

	companyID, err := strconv.ParseInt(c.Query("company_id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "company_id is required"})
		return
	}

	owned, err := companyBelongsToUser(c.Request.Context(),
		h.db, companyID, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to verify company"})
		return
	}
	if !owned {
		c.JSON(http.StatusForbidden, gin.H{"error": "unauthorized"})
		return
	}

	summary, err := h.service.Summary(c.Request.Context(),
		companyID, strings.TrimSpace(c.Query("search")))
	if err != nil {
		log.Println("failed to fetch credit note summary:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch summary"})
		return
	}

	c.JSON(http.StatusOK, summary)
}

// ApplyToInvoice puts a credit note's balance against one of the customer's invoices.
//
// POST /api/v1/credit-notes/:id/apply?company_id=1
//
// Body: {"invoice_id": 42, "amount": 500}. Amount may be left out, which applies as much
// of the credit as the invoice can take — what somebody means by "apply this to that".
func (h *CreditNoteHandler) ApplyToInvoice(c *gin.Context) {
	companyID, err := strconv.ParseInt(c.Query("company_id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "company_id is required"})
		return
	}
	owned, err := companyBelongsToUser(c.Request.Context(),
		h.db, companyID, c.GetInt("user_id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to verify company"})
		return
	}
	if !owned {
		c.JSON(http.StatusForbidden, gin.H{"error": "Unauthorized company access"})
		return
	}

	creditNoteID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid credit note"})
		return
	}

	var req struct {
		InvoiceID int64   `json:"invoice_id"`
		Amount    float64 `json:"amount"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid input"})
		return
	}
	if req.InvoiceID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Choose an invoice to put it against."})
		return
	}

	applied, err := h.service.ApplyToInvoice(c.Request.Context(),
		companyID, creditNoteID, req.InvoiceID, req.Amount)
	if err != nil {
		var input services.CreditNoteInputError
		if errors.As(err, &input) {
			c.JSON(http.StatusBadRequest, gin.H{"error": input.Msg})
			return
		}
		log.Println("failed to apply credit note:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to apply that credit"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Credit applied", "applied": applied})
}
