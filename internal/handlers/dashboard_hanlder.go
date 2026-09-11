// internal/handlers/dashboard.go

package handlers

import (
	"log"
	"net/http"
	"strconv"

	database "invo-server/internal/db"
	"invo-server/internal/models"
	utils "invo-server/internal/util"

	"github.com/gin-gonic/gin"
	"golang.org/x/sync/errgroup"
)

type DashboardHandler struct {
	db *database.Database
}

func NewDashboardHandler(db *database.Database) *DashboardHandler {
	return &DashboardHandler{db: db}
}

func (h *DashboardHandler) GetDashboard(c *gin.Context) {

	userID := c.GetInt("user_id")
	companyID := c.Query("companyId")

	companyIDInt, err := strconv.ParseInt(companyID, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid companyId"})
		return
	}
	owned, err := companyBelongsToUser(h.db.DB, companyIDInt, userID)
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
	prevStart, prevEnd := utils.PreviousPeriod(period, start)

	var resp models.DashboardResponse
	resp.Period = period

	var prevRevenue float64

	g, ctx := errgroup.WithContext(c.Request.Context())

	// 1️⃣ Revenue (current period)
	g.Go(func() error {
		return h.db.DB.QueryRowContext(ctx, `
			SELECT COALESCE(SUM(total),0)
			FROM invoices
			WHERE company_id = $1
			  AND status IN ('paid','partial','issued')
			  AND invoice_date BETWEEN $2 AND $3
		`, companyID, start, end).Scan(&resp.Revenue.Total)
	})

	// 2️⃣ Revenue (previous period)
	g.Go(func() error {
		return h.db.DB.QueryRowContext(ctx, `
			SELECT COALESCE(SUM(total),0)
			FROM invoices
			WHERE company_id = $1
			  AND status IN ('paid','partial','issued')
			  AND invoice_date BETWEEN $2 AND $3
		`, companyID, prevStart, prevEnd).Scan(&prevRevenue)
	})

	// 3️⃣ Counts
	g.Go(func() error {
		return h.db.DB.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM invoices WHERE company_id = $1
		`, companyID).Scan(&resp.Counts.Invoices)
	})

	g.Go(func() error {
		return h.db.DB.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM clients WHERE company_id = $1 AND user_id = $2
		`, companyID, userID).Scan(&resp.Counts.Clients)
	})

	g.Go(func() error {
		return h.db.DB.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM items WHERE company_id = $1 AND user_id = $2
		`, companyID, userID).Scan(&resp.Counts.Items)
	})

	// 4️⃣ Recent invoices
	g.Go(func() error {
		rows, err := h.db.DB.QueryContext(ctx, `
			SELECT
				i.id,
				i.invoice_number,
				c.name,
				i.total,
				i.status,
				i.created_at
			FROM invoices i
			JOIN clients c ON c.id = i.client_id
			WHERE i.company_id = $1
			ORDER BY i.created_at DESC
			LIMIT 5
		`, companyID)
		if err != nil {
			return err
		}
		defer rows.Close()

		recent := []models.RecentInvoice{}
		for rows.Next() {
			var inv models.RecentInvoice
			if err := rows.Scan(
				&inv.ID,
				&inv.InvoiceNo,
				&inv.ClientName,
				&inv.Total,
				&inv.Status,
				&inv.CreatedAt,
			); err != nil {
				return err
			}
			recent = append(recent, inv)
		}
		resp.Recent = recent
		return rows.Err()
	})

	if err := g.Wait(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load dashboard"})
		return
	}

	if prevRevenue > 0 {
		resp.Revenue.ChangePercent =
			((resp.Revenue.Total - prevRevenue) / prevRevenue) * 100
	}

	// Last seven days of revenue for the dashboard chart, which drew hardcoded bar
	// heights before this existed. generate_series supplies the days so a day with no
	// sales appears as a zero bar rather than being missing from the series, which
	// would otherwise compress the axis and misrepresent the shape.
	trendRows, err := h.db.DB.Query(`
		SELECT TO_CHAR(d.day, 'YYYY-MM-DD'), COALESCE(SUM(i.total), 0)
		FROM generate_series(CURRENT_DATE - INTERVAL '6 days', CURRENT_DATE, INTERVAL '1 day') AS d(day)
		LEFT JOIN invoices i
		       ON i.invoice_date = d.day::date
		      AND i.company_id = $1
		      AND i.status NOT IN ('draft', 'cancelled')
		GROUP BY d.day
		ORDER BY d.day
	`, companyID)
	if err != nil {
		log.Println("failed to load revenue trend:", err)
	} else {
		defer trendRows.Close()
		for trendRows.Next() {
			var point models.DailyRevenue
			if err := trendRows.Scan(&point.Date, &point.Total); err != nil {
				log.Println("failed to scan revenue trend:", err)
				break
			}
			resp.Revenue.Trend = append(resp.Revenue.Trend, point)
		}
	}

	c.JSON(http.StatusOK, resp)
}
