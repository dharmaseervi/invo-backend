package handlers

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"invo-server/internal/pdf"
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
	owned, err := companyBelongsToUser(c.Request.Context(),
		h.db, companyID, userID)
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
	owned, err := companyBelongsToUser(c.Request.Context(),
		h.db, companyID, userID)
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
	owned, err := companyBelongsToUser(c.Request.Context(),
		h.db, companyID, userID)
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
	owned, err := companyBelongsToUser(c.Request.Context(),
		h.db, companyID, c.GetInt("user_id"))
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

// GetClientStatementPDF renders a customer's statement of account as a PDF.
//
// GET /api/v1/ledger/:clientId/statement.pdf?company_id=1&start=&end=
//
// The app already shows this ledger on screen. This is the version that gets sent to
// the customer when they ring up saying they have paid everything — one page, opening
// balance, every movement, closing figure.
//
// Defaults to the current financial year, which in India starts in April and is the
// period a shopkeeper means by "this year" without having to say so.
func (h *LedgerHandler) GetClientStatementPDF(c *gin.Context) {
	clientID, err := strconv.ParseInt(c.Param("clientId"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid client id"})
		return
	}

	// The header is how the app's other ledger calls pass this; the query string is
	// accepted too, because a PDF is also something somebody opens in a browser where
	// setting a header is not on offer.
	companyIDStr := c.GetHeader("X-Company-ID")
	if companyIDStr == "" {
		companyIDStr = c.Query("company_id")
	}
	companyID, err := strconv.ParseInt(companyIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "company_id missing"})
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

	from, to := statementPeriod(c.Query("start"), c.Query("end"))

	statement, err := h.ledgerService.ClientStatementFor(c.Request.Context(), companyID, clientID, from, to)
	if err != nil {
		var input services.LedgerInputError
		if errors.As(err, &input) {
			c.JSON(http.StatusBadRequest, gin.H{"error": input.Msg})
			return
		}
		log.Println("failed to build statement:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to build that statement"})
		return
	}

	data := pdf.StatementData{
		CompanyName:    statement.CompanyName,
		CompanyGSTIN:   statement.CompanyGSTIN,
		CompanyPhone:   statement.CompanyPhone,
		CompanyAddress: statement.CompanyAddress,
		ClientName:     statement.ClientName,
		ClientPhone:    statement.ClientPhone,
		ClientAddress:  statement.ClientAddress,
		From:           statement.From,
		To:             statement.To,
		Opening:        statement.Opening,
		Billed:         statement.Billed,
		Paid:           statement.Paid,
		Closing:        statement.Closing,
	}
	for _, entry := range statement.Entries {
		data.Lines = append(data.Lines, pdf.StatementLine{
			Date:        entry.CreatedAt,
			Description: entry.Description,
			Reference:   entry.SourceType,
			Debit:       entry.Debit,
			Credit:      entry.Credit,
			Balance:     entry.Balance,
		})
	}

	bytes, err := pdf.GenerateStatementPDF(data)
	if err != nil {
		log.Println("failed to render statement pdf:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to build that statement"})
		return
	}

	// Named after the customer and the period, because these get saved and forwarded
	// and a folder of Statement.pdf tells nobody anything.
	fileName := fmt.Sprintf(
		"Statement_%s_%s.pdf",
		safeFileName(statement.ClientName), from.Format("Jan2006"),
	)

	c.Header("Content-Type", "application/pdf")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", fileName))
	c.Header("Content-Length", fmt.Sprintf("%d", len(bytes)))
	c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
	c.Data(http.StatusOK, "application/pdf", bytes)
}

// statementPeriod reads the dates asked for, falling back to the Indian financial year
// in progress: April to March, which is what "this year" means to a shop here.
func statementPeriod(startText, endText string) (time.Time, time.Time) {
	const layout = "2006-01-02"

	now := time.Now()
	yearStart := time.Date(now.Year(), time.April, 1, 0, 0, 0, 0, now.Location())
	if now.Before(yearStart) {
		yearStart = yearStart.AddDate(-1, 0, 0)
	}

	from := yearStart
	if parsed, err := time.ParseInLocation(layout, startText, now.Location()); err == nil {
		from = parsed
	}

	to := now
	if parsed, err := time.ParseInLocation(layout, endText, now.Location()); err == nil {
		to = parsed
	}

	// A period that runs backwards produces an empty statement that looks like an
	// account with no history, so it is turned the right way round instead.
	if to.Before(from) {
		from, to = to, from
	}
	return from, to
}

// safeFileName keeps a customer's name usable as a filename on any phone it lands on.
func safeFileName(name string) string {
	cleaned := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			return r
		case r == ' ', r == '-', r == '_':
			return '_'
		default:
			return -1
		}
	}, name)
	cleaned = strings.Trim(cleaned, "_")
	if cleaned == "" {
		return "Customer"
	}
	if len([]rune(cleaned)) > 40 {
		cleaned = string([]rune(cleaned)[:40])
	}
	return cleaned
}
