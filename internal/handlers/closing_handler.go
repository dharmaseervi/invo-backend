package handlers

import (
	"database/sql"
	"errors"
	"log"
	"net/http"
	"strconv"
	"time"

	"invo-server/internal/services"

	"github.com/gin-gonic/gin"
)

// Closing the day: counting the floor, and counting the drawer.

type ClosingHandler struct {
	db        *sql.DB
	stocktake *services.StocktakeService
	ledger    *services.LedgerService
}

func NewClosingHandler(
	db *sql.DB,
	stocktake *services.StocktakeService,
	ledger *services.LedgerService,
) *ClosingHandler {
	return &ClosingHandler{db: db, stocktake: stocktake, ledger: ledger}
}

func (h *ClosingHandler) company(c *gin.Context) (int64, bool) {
	companyID, err := strconv.ParseInt(c.Query("company_id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "company_id is required"})
		return 0, false
	}
	owned, err := companyBelongsToUser(h.db, companyID, c.GetInt("user_id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to verify company"})
		return 0, false
	}
	if !owned {
		c.JSON(http.StatusForbidden, gin.H{"error": "Unauthorized company access"})
		return 0, false
	}
	return companyID, true
}

// failClosing reports a fixable problem word for word and anything else as a failure.
func failClosing(c *gin.Context, err error, generic string) {
	var stocktakeErr services.StocktakeInputError
	if errors.As(err, &stocktakeErr) {
		c.JSON(http.StatusBadRequest, gin.H{"error": stocktakeErr.Msg})
		return
	}
	var ledgerErr services.LedgerInputError
	if errors.As(err, &ledgerErr) {
		c.JSON(http.StatusBadRequest, gin.H{"error": ledgerErr.Msg})
		return
	}
	log.Println(generic+":", err)
	c.JSON(http.StatusInternalServerError, gin.H{"error": generic})
}

// MARK: - Stocktake

// StartStocktake opens a count, or hands back the one already open.
//
// POST /api/v1/stocktakes?company_id=1
func (h *ClosingHandler) StartStocktake(c *gin.Context) {
	companyID, ok := h.company(c)
	if !ok {
		return
	}

	var req struct {
		Note string `json:"note"`
	}
	_ = c.ShouldBindJSON(&req)

	id, err := h.stocktake.Start(c.Request.Context(), companyID, int64(c.GetInt("user_id")), req.Note)
	if err != nil {
		failClosing(c, err, "Failed to start that count")
		return
	}
	c.JSON(http.StatusOK, gin.H{"stocktake_id": id})
}

// CountItem records what was found for one item.
//
// POST /api/v1/stocktakes/:id/count?company_id=1
func (h *ClosingHandler) CountItem(c *gin.Context) {
	companyID, ok := h.company(c)
	if !ok {
		return
	}
	stocktakeID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid count"})
		return
	}

	var req struct {
		ItemID  int64 `json:"item_id"`
		Counted int   `json:"counted"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid input"})
		return
	}

	if err := h.stocktake.Count(c.Request.Context(), companyID, stocktakeID, req.ItemID, req.Counted); err != nil {
		failClosing(c, err, "Failed to record that count")
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Counted"})
}

// GetStocktake reads a count and its lines.
//
// GET /api/v1/stocktakes/:id?company_id=1
func (h *ClosingHandler) GetStocktake(c *gin.Context) {
	companyID, ok := h.company(c)
	if !ok {
		return
	}
	stocktakeID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid count"})
		return
	}

	stocktake, err := h.stocktake.Get(c.Request.Context(), companyID, stocktakeID)
	if err != nil {
		failClosing(c, err, "Failed to load that count")
		return
	}
	c.JSON(http.StatusOK, stocktake)
}

// ApplyStocktake writes the count into the stock figures.
//
// POST /api/v1/stocktakes/:id/apply?company_id=1
func (h *ClosingHandler) ApplyStocktake(c *gin.Context) {
	companyID, ok := h.company(c)
	if !ok {
		return
	}
	stocktakeID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid count"})
		return
	}

	adjusted, err := h.stocktake.Apply(c.Request.Context(), companyID, int64(c.GetInt("user_id")), stocktakeID)
	if err != nil {
		failClosing(c, err, "Failed to finish that count")
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Stock updated", "items_adjusted": adjusted})
}

// AbandonStocktake throws a count away without touching stock.
//
// DELETE /api/v1/stocktakes/:id?company_id=1
func (h *ClosingHandler) AbandonStocktake(c *gin.Context) {
	companyID, ok := h.company(c)
	if !ok {
		return
	}
	stocktakeID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid count"})
		return
	}

	if err := h.stocktake.Abandon(c.Request.Context(), companyID, stocktakeID); err != nil {
		failClosing(c, err, "Failed to discard that count")
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Discarded"})
}

// MARK: - Cash

// GetDayClosing is what the drawer should hold for a day, and what was counted if it
// has been closed.
//
// GET /api/v1/day-closing?company_id=1&date=YYYY-MM-DD
func (h *ClosingHandler) GetDayClosing(c *gin.Context) {
	companyID, ok := h.company(c)
	if !ok {
		return
	}

	closing, err := h.ledger.ClosingFor(c.Request.Context(), companyID, closingDate(c.Query("date")))
	if err != nil {
		failClosing(c, err, "Failed to load that day")
		return
	}
	c.JSON(http.StatusOK, closing)
}

// CloseDay records what was counted out of the drawer.
//
// POST /api/v1/day-closing?company_id=1
func (h *ClosingHandler) CloseDay(c *gin.Context) {
	companyID, ok := h.company(c)
	if !ok {
		return
	}

	var req struct {
		Date    string  `json:"date"`
		Counted float64 `json:"counted_cash"`
		// Optional: left out, the drawer carries over from the last closing.
		Opening *float64 `json:"opening_cash"`
		Note    string   `json:"note"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid input"})
		return
	}

	closing, err := h.ledger.Close(
		c.Request.Context(), companyID, int64(c.GetInt("user_id")),
		closingDate(req.Date), req.Counted, req.Opening, req.Note,
	)
	if err != nil {
		failClosing(c, err, "Failed to close that day")
		return
	}
	c.JSON(http.StatusOK, closing)
}

// GetRecentClosings is the last few days of cash counts.
//
// GET /api/v1/day-closings?company_id=1&limit=30
func (h *ClosingHandler) GetRecentClosings(c *gin.Context) {
	companyID, ok := h.company(c)
	if !ok {
		return
	}

	closings, err := h.ledger.RecentClosings(c.Request.Context(), companyID, mustAtoi(c.Query("limit")))
	if err != nil {
		failClosing(c, err, "Failed to load those days")
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": closings})
}

// closingDate reads a date, defaulting to today. An unreadable one becomes today too:
// the shop is closing the day it is standing in, and refusing over a malformed
// parameter helps nobody.
func closingDate(value string) string {
	if parsed, err := time.Parse("2006-01-02", value); err == nil {
		return parsed.Format("2006-01-02")
	}
	return time.Now().Format("2006-01-02")
}
