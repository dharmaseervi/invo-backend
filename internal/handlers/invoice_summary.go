package handlers

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

// invoiceStatusClause turns a status filter into SQL, or "" for no filter.
//
// "overdue" and "owed" are not stored statuses. They are the two questions a list is
// actually asked — who is late, and who owes anything at all — and they have to be
// answered the same way everywhere, which is why the SQL lives in one place rather than
// being rewritten per endpoint.
func invoiceStatusClause(status, alias string) string {
	switch status {
	case "draft", "issued", "partial", "paid", "cancelled":
		// Interpolated, but only from the fixed list above — never from the raw query.
		return " AND " + alias + ".status = '" + status + "'"
	case "owed":
		return " AND " + alias + ".status IN ('issued', 'partial')"
	case "overdue":
		return " AND " + alias + ".status IN ('issued', 'partial') AND CURRENT_DATE > " + alias + ".due_date"
	default:
		return ""
	}
}

// GetInvoiceSummary returns the figures a list screen shows above the rows: what is
// outstanding, and how many invoices sit in each state.
//
// GET /api/v1/invoices/summary?company_id=&client_id=&search=
//
// The apps worked these out from the rows they had loaded, which is the first page.
// "Outstanding" was therefore the outstanding amount of the most recent ten invoices,
// and the count beside each filter described those ten — presented as the state of the
// business. The database can answer it over everything, in one query, whatever the page
// size is.
func (h *InvoiceHandler) GetInvoiceSummary(c *gin.Context) {
	userID := c.GetInt("user_id")

	args := []interface{}{userID}
	argPos := 2
	where := " WHERE i.company_id IN (SELECT company_id FROM companies_for_user($1))"

	if v := c.Query("company_id"); v != "" {
		if companyID, err := strconv.Atoi(v); err == nil {
			owned, err := companyBelongsToUser(h.db.DB, int64(companyID), userID)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to verify company"})
				return
			}
			if !owned {
				c.JSON(http.StatusForbidden, gin.H{"error": "Unauthorized company"})
				return
			}
			where += " AND i.company_id = $" + strconv.Itoa(argPos)
			args = append(args, companyID)
			argPos++
		}
	}

	if v := c.Query("client_id"); v != "" {
		if clientID, err := strconv.Atoi(v); err == nil {
			where += " AND i.client_id = $" + strconv.Itoa(argPos)
			args = append(args, clientID)
			argPos++
		}
	}

	// The same search the list applies, so the figures describe the rows on screen.
	if search := strings.TrimSpace(c.Query("search")); search != "" {
		where += " AND (i.invoice_number ILIKE $" + strconv.Itoa(argPos) +
			" OR c.name ILIKE $" + strconv.Itoa(argPos) + ")"
		args = append(args, "%"+search+"%")
		argPos++
	}

	var s struct {
		Total     int `json:"total"`
		Draft     int `json:"draft"`
		Issued    int `json:"issued"`
		Partial   int `json:"partial"`
		Paid      int `json:"paid"`
		Cancelled int `json:"cancelled"`
		Overdue   int `json:"overdue"`
		Owed      int `json:"owed"`
		// Outstanding counts issued and part-paid invoices only: a draft has not been
		// sent to anybody and a cancelled one is void, so neither is owed.
		Outstanding   float64 `json:"outstanding"`
		OverdueAmount float64 `json:"overdue_amount"`
		// What the matching invoices came to in total, drafts and cancellations aside.
		Invoiced float64 `json:"invoiced"`
	}

	err := h.db.DB.QueryRow(`
		SELECT
			COUNT(*),
			COUNT(*) FILTER (WHERE i.status = 'draft'),
			COUNT(*) FILTER (WHERE i.status = 'issued'),
			COUNT(*) FILTER (WHERE i.status = 'partial'),
			COUNT(*) FILTER (WHERE i.status = 'paid'),
			COUNT(*) FILTER (WHERE i.status = 'cancelled'),
			COUNT(*) FILTER (WHERE i.status IN ('issued','partial') AND CURRENT_DATE > i.due_date),
			COUNT(*) FILTER (WHERE i.status IN ('issued','partial')),
			COALESCE(SUM(i.remaining_amount) FILTER (WHERE i.status IN ('issued','partial')), 0),
			COALESCE(SUM(i.remaining_amount) FILTER (WHERE i.status IN ('issued','partial') AND CURRENT_DATE > i.due_date), 0),
			COALESCE(SUM(i.total) FILTER (WHERE i.status <> 'cancelled' AND i.status <> 'draft'), 0)
		FROM invoices i
		JOIN clients c ON c.id = i.client_id
	`+where, args...).Scan(
		&s.Total, &s.Draft, &s.Issued, &s.Partial, &s.Paid, &s.Cancelled,
		&s.Overdue, &s.Owed, &s.Outstanding, &s.OverdueAmount, &s.Invoiced,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch summary"})
		return
	}

	c.JSON(http.StatusOK, s)
}
