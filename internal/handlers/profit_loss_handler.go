package handlers

import (
	"database/sql"
	"net/http"
	"strconv"
	"time"

	utils "invo-server/internal/util"

	"github.com/gin-gonic/gin"
	"golang.org/x/sync/errgroup"
)

type ProfitLossHandler struct {
	db *sql.DB
}

func NewProfitLossHandler(db *sql.DB) *ProfitLossHandler {
	return &ProfitLossHandler{db: db}
}

type ProfitLossResponse struct {
	Period       string  `json:"period"`
	From         string  `json:"from"`
	To           string  `json:"to"`
	Revenue      float64 `json:"revenue"`
	Purchases    float64 `json:"purchases"`
	Expenses     float64 `json:"expenses"`
	GrossProfit  float64 `json:"gross_profit"`
	NetProfit    float64 `json:"net_profit"`
	GrossMargin  float64 `json:"gross_margin_pct"`
	NetMargin    float64 `json:"net_margin_pct"`
	InvoiceCount int     `json:"invoice_count"`
	BillCount    int     `json:"bill_count"`
	ExpenseCount int     `json:"expense_count"`
}

// GET /api/v1/companies/:companyId/reports/profit-loss?period=week|month|quarter|year|financial-year
func (h *ProfitLossHandler) GetProfitLoss(c *gin.Context) {
	companyID, err := strconv.ParseInt(c.Param("companyId"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid companyId"})
		return
	}

	userID := c.GetInt("user_id")
	owned, err := companyBelongsToUser(c.Request.Context(), h.db, companyID, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to verify company"})
		return
	}
	if !owned {
		c.JSON(http.StatusForbidden, gin.H{"error": "Unauthorized"})
		return
	}

	period := c.DefaultQuery("period", "month")
	start, end := utils.PeriodRange(period)

	var resp ProfitLossResponse
	resp.Period = period
	resp.From = start.Format(time.DateOnly)
	resp.To = end.Format(time.DateOnly)

	g, ctx := errgroup.WithContext(c.Request.Context())

	// Revenue: all non-cancelled, non-draft invoices dated in the period.
	// Accrual basis — what was billed, not just what was collected.
	g.Go(func() error {
		return h.db.QueryRowContext(ctx, `
			SELECT COALESCE(SUM(total), 0), COUNT(*)
			FROM invoices
			WHERE company_id = $1
			  AND status NOT IN ('draft', 'cancelled')
			  AND invoice_date BETWEEN $2 AND $3
		`, companyID, start, end).Scan(&resp.Revenue, &resp.InvoiceCount)
	})

	// Purchases: non-cancelled supplier bills.
	g.Go(func() error {
		return h.db.QueryRowContext(ctx, `
			SELECT COALESCE(SUM(total), 0), COUNT(*)
			FROM purchase_bills
			WHERE company_id = $1
			  AND status <> 'cancelled'
			  AND bill_date BETWEEN $2 AND $3
		`, companyID, start, end).Scan(&resp.Purchases, &resp.BillCount)
	})

	// Expenses: what the shop spent running itself.
	g.Go(func() error {
		return h.db.QueryRowContext(ctx, `
			SELECT COALESCE(SUM(amount), 0), COUNT(*)
			FROM expensess
			WHERE company_id = $1
			  AND date BETWEEN $2 AND $3
		`, companyID, start, end).Scan(&resp.Expenses, &resp.ExpenseCount)
	})

	if err := g.Wait(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate report"})
		return
	}

	resp.GrossProfit = resp.Revenue - resp.Purchases
	resp.NetProfit = resp.GrossProfit - resp.Expenses

	if resp.Revenue > 0 {
		resp.GrossMargin = (resp.GrossProfit / resp.Revenue) * 100
		resp.NetMargin = (resp.NetProfit / resp.Revenue) * 100
	}

	c.JSON(http.StatusOK, resp)
}
