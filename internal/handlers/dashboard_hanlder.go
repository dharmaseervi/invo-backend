// internal/handlers/dashboard.go

package handlers

import (
	"net/http"

	database "invo-server/internal/db"
	"invo-server/internal/models"
	utils "invo-server/internal/util"

	"github.com/gin-gonic/gin"
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

	period := c.DefaultQuery("period", "month")
	start, end := utils.PeriodRange(period)

	var resp models.DashboardResponse
	resp.Period = period

	/* -----------------------------
	   1️⃣ Revenue (current period)
	------------------------------ */
	err := h.db.DB.QueryRow(`
		SELECT COALESCE(SUM(total),0)
		FROM invoices
		WHERE company_id = $1
		  AND status IN ('paid','partial','issued')
		  AND invoice_date BETWEEN $2 AND $3
	`, companyID, start, end).Scan(&resp.Revenue.Total)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed revenue"})
		return
	}

	/* -----------------------------
	   2️⃣ Revenue (previous period)
	------------------------------ */
	prevStart, prevEnd := utils.PreviousPeriod(period, start)

	var prevRevenue float64
	_ = h.db.DB.QueryRow(`
		SELECT COALESCE(SUM(total),0)
		FROM invoices
		WHERE company_id = $1
		  AND status IN ('paid','partial','issued')
		  AND invoice_date BETWEEN $2 AND $3
	`, companyID, prevStart, prevEnd).Scan(&prevRevenue)

	if prevRevenue > 0 {
		resp.Revenue.ChangePercent =
			((resp.Revenue.Total - prevRevenue) / prevRevenue) * 100
	}

	/* -----------------------------
	   3️⃣ Counts
	------------------------------ */
	_ = h.db.DB.QueryRow(`
		SELECT COUNT(*) FROM invoices WHERE company_id = $1
	`, companyID).Scan(&resp.Counts.Invoices)

	_ = h.db.DB.QueryRow(`
		SELECT COUNT(*) FROM clients WHERE company_id = $1 AND user_id = $2
	`, companyID, userID).Scan(&resp.Counts.Clients)

	_ = h.db.DB.QueryRow(`
		SELECT COUNT(*) FROM items WHERE company_id = $1 AND user_id = $2
	`, companyID, userID).Scan(&resp.Counts.Items)

	/* -----------------------------
	   4️⃣ Recent invoices
	------------------------------ */
	rows, err := h.db.DB.Query(`
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed recent"})
		return
	}
	defer rows.Close()

	for rows.Next() {
		var inv models.RecentInvoice
		rows.Scan(
			&inv.ID,
			&inv.InvoiceNo,
			&inv.ClientName,
			&inv.Total,
			&inv.Status,
			&inv.CreatedAt,
		)
		resp.Recent = append(resp.Recent, inv)
	}

	c.JSON(http.StatusOK, resp)
}
