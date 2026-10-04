package handlers

import (
	"database/sql"
	"log"
	"net/http"
	"strconv"
	"strings"

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
		clampPageSize(mustAtoi(c.Query("limit")), 0),
		mustAtoi(c.Query("offset")),
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
		clampPageSize(mustAtoi(c.Query("limit")), 0),
		mustAtoi(c.Query("offset")),
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

// GetClientLedgerSummary returns one customer's totals over their whole history.
//
// GET /api/v1/ledger/:clientId/summary with X-Company-ID
//
// The statement screen used to add up every row it had fetched. Paging those rows
// would have made those totals the totals of a page, so the figures come from here
// instead and the rows can be fetched a page at a time.
func (h *LedgerHandler) GetClientLedgerSummary(c *gin.Context) {
	clientID, err := strconv.ParseInt(c.Param("clientId"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid client id"})
		return
	}

	companyID, ok := h.companyFromHeader(c)
	if !ok {
		return
	}

	summary, err := h.ledgerService.ClientLedgerSummary(c.Request.Context(), companyID, clientID)
	if err != nil {
		log.Println("failed to fetch client ledger summary:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch ledger summary"})
		return
	}

	c.JSON(http.StatusOK, summary)
}

// GetCompanyLedgerSummaries returns one row per customer with ledger history: the list
// a ledger screen shows, without downloading the business's entire history to build it.
//
// GET /api/v1/companies/:companyId/ledger/summary?search=&limit=&offset=
func (h *LedgerHandler) GetCompanyLedgerSummaries(c *gin.Context) {
	companyID, err := strconv.ParseInt(c.Param("companyId"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid company id"})
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

	rows, err := h.ledgerService.CompanyLedgerSummaries(
		c.Request.Context(),
		companyID,
		strings.TrimSpace(c.Query("search")),
		clampPageSize(mustAtoi(c.Query("limit")), 0),
		mustAtoi(c.Query("offset")),
	)
	if err != nil {
		log.Println("failed to fetch company ledger summaries:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch ledger summary"})
		return
	}

	totals, err := h.ledgerService.CompanyLedgerTotals(
		c.Request.Context(), companyID, strings.TrimSpace(c.Query("search")),
	)
	if err != nil {
		log.Println("failed to fetch company ledger totals:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch ledger summary"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": rows, "totals": totals})
}

// companyFromHeader reads and authorises X-Company-ID, which is how the ledger routes
// scope a request. It writes the response and returns false when the caller must be
// refused.
func (h *LedgerHandler) companyFromHeader(c *gin.Context) (int64, bool) {
	raw := c.GetHeader("X-Company-ID")
	if raw == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "company_id missing"})
		return 0, false
	}
	companyID, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid company_id"})
		return 0, false
	}
	owned, err := companyBelongsToUser(h.db, companyID, c.GetInt("user_id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to verify company"})
		return 0, false
	}
	if !owned {
		c.JSON(http.StatusForbidden, gin.H{"error": "Unauthorized"})
		return 0, false
	}
	return companyID, true
}
