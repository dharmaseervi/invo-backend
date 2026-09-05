package handlers

import (
	"database/sql"
	"log"
	"net/http"
	"strconv"

	"invo-server/internal/services"

	"github.com/gin-gonic/gin"
)

type LedgerHandler struct {
	ledgerService *services.LedgerService
	db            *sql.DB
}

func NewLedgerHandler(ls *services.LedgerService, db *sql.DB) *LedgerHandler {
	return &LedgerHandler{ledgerService: ls, db: db}
}

// GET /api/v1/ledger/:clientId
func (h *LedgerHandler) GetClientLedger(c *gin.Context) {
	clientID, err := strconv.ParseInt(c.Param("clientId"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid client id"})
		return
	}

	companyIDStr := c.GetHeader("X-Company-ID")
	if companyIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "company_id missing"})
		return
	}

	companyID, err := strconv.ParseInt(companyIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid company_id"})
		return
	}

	userID := c.GetInt("user_id")
	owned, err := companyBelongsToUser(h.db, companyID, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to verify company"})
		return
	}
	if !owned {
		c.JSON(http.StatusForbidden, gin.H{"error": "Unauthorized"})
		return
	}

	entries, err := h.ledgerService.GetClientLedger(
		c.Request.Context(),
		companyID,
		clientID,
	)

	if err != nil {
		log.Println("failed to fetch client ledger:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch ledger"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": entries,
	})
}

// GET /api/v1/ledger
func (h *LedgerHandler) GetCompanyLedger(c *gin.Context) {

	companyID, err := strconv.ParseInt(
		c.Param("companyId"), 10, 64,
	)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid company id",
		})
		return
	}

	userID := c.GetInt("user_id")
	owned, err := companyBelongsToUser(h.db, companyID, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to verify company"})
		return
	}
	if !owned {
		c.JSON(http.StatusForbidden, gin.H{"error": "Unauthorized"})
		return
	}

	entries, err := h.ledgerService.GetCompanyLedger(
		c.Request.Context(),
		companyID,
	)
	if err != nil {
		log.Println("failed to fetch company ledger:", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to fetch ledger",
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"data": entries,
	})
}
