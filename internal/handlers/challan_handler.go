package handlers

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	database "invo-server/internal/db"
	"invo-server/internal/models"
)

type ChallanHandler struct {
	db *database.Database
}

func NewChallanHandler(db *database.Database) *ChallanHandler {
	return &ChallanHandler{db: db}
}

// POST /challans
func (h *ChallanHandler) CreateChallan(c *gin.Context) {
	userID := c.GetInt("user_id")

	var req models.ChallanRequestDTO
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Verify company belongs to user
	var owned bool
	err := h.db.DB.QueryRowContext(c.Request.Context(),
		`SELECT EXISTS (SELECT 1 FROM companies_for_user($1) WHERE company_id = $2)`,
		userID, req.CompanyID).Scan(&owned)
	if err != nil || !owned {
		c.JSON(http.StatusForbidden, gin.H{"error": "Unauthorized"})
		return
	}

	if req.CompanyID <= 0 || req.ClientID <= 0 || len(req.Items) == 0 || len(req.Items) > 500 {
		c.JSON(400, gin.H{"error": "A company, customer and 1–500 items are required"})
		return
	}
	if _, err := time.Parse("2006-01-02", req.ChallanDate); err != nil {
		c.JSON(400, gin.H{"error": "Use a valid challan date (YYYY-MM-DD)"})
		return
	}
	if len(req.Notes) > 4000 {
		c.JSON(400, gin.H{"error": "Notes must be at most 4000 characters"})
		return
	}
	for _, item := range req.Items {
		if item.ItemID <= 0 || item.Qty <= 0 || item.Qty > 1000000000 || len(item.Description) > 2000 {
			c.JSON(400, gin.H{"error": "Items require a positive quantity and a description of at most 2000 characters"})
			return
		}
	}
	tx, err := h.db.DB.BeginTx(c.Request.Context(), nil)
	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to start transaction"})
		return
	}
	defer tx.Rollback()
	var refID int
	if err = tx.QueryRowContext(c.Request.Context(), `SELECT id FROM clients WHERE id=$1 AND company_id=$2 FOR KEY SHARE`, req.ClientID, req.CompanyID).Scan(&refID); err != nil {
		c.JSON(400, gin.H{"error": "Customer is not available in this company"})
		return
	}
	if req.InvoiceID != nil {
		if err = tx.QueryRowContext(c.Request.Context(), `SELECT id FROM invoices WHERE id=$1 AND company_id=$2 AND client_id=$3 FOR KEY SHARE`, *req.InvoiceID, req.CompanyID, req.ClientID).Scan(&refID); err != nil {
			c.JSON(400, gin.H{"error": "Invoice must belong to this company and customer"})
			return
		}
	}
	for _, item := range req.Items {
		if err = tx.QueryRowContext(c.Request.Context(), `SELECT id FROM items WHERE id=$1 AND company_id=$2 FOR KEY SHARE`, item.ItemID, req.CompanyID).Scan(&refID); err != nil {
			c.JSON(400, gin.H{"error": "An item is not available in this company"})
			return
		}
	}
	var nextNum int
	if err = tx.QueryRowContext(c.Request.Context(), `SELECT nextval('delivery_challan_seq')`).Scan(&nextNum); err != nil {
		c.JSON(500, gin.H{"error": "Failed to generate challan number"})
		return
	}
	challanNumber := fmt.Sprintf("DC/%04d", nextNum)

	var challanID int
	err = tx.QueryRowContext(c.Request.Context(),
		`INSERT INTO delivery_challans (company_id, client_id, challan_number, challan_date, invoice_id, notes)
		 VALUES ($1,$2,$3,$4,$5,$6) RETURNING id`,
		req.CompanyID, req.ClientID, challanNumber, req.ChallanDate, req.InvoiceID, req.Notes,
	).Scan(&challanID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create challan"})
		return
	}

	for _, item := range req.Items {
		_, err = tx.ExecContext(c.Request.Context(),
			`INSERT INTO delivery_challan_items (challan_id, item_id, qty, description, company_id) VALUES ($1,$2,$3,$4,$5)`,
			challanID, item.ItemID, item.Qty, item.Description, req.CompanyID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to add challan item"})
			return
		}
	}

	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to commit"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"id": challanID, "challan_number": challanNumber})
}

// GET /challans?company_id=X
func (h *ChallanHandler) ListChallans(c *gin.Context) {
	userID := c.GetInt("user_id")
	companyID, err := strconv.Atoi(c.Query("company_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "company_id required"})
		return
	}

	limit, offset := 50, 0
	if value := c.Query("limit"); value != "" {
		limit, err = strconv.Atoi(value)
		if err != nil || limit < 1 || limit > 100 {
			c.JSON(400, gin.H{"error": "limit must be 1–100"})
			return
		}
	}
	if value := c.Query("offset"); value != "" {
		offset, err = strconv.Atoi(value)
		if err != nil || offset < 0 {
			c.JSON(400, gin.H{"error": "offset must be nonnegative"})
			return
		}
	}
	search := strings.TrimSpace(c.Query("search"))
	if len(search) > 200 {
		c.JSON(400, gin.H{"error": "Search is too long"})
		return
	}
	var owned bool
	if err = h.db.DB.QueryRowContext(c.Request.Context(), `SELECT EXISTS(SELECT 1 FROM companies_for_user($1) WHERE company_id=$2)`, userID, companyID).Scan(&owned); err != nil {
		c.JSON(500, gin.H{"error": "Failed to verify access"})
		return
	}
	if !owned {
		c.JSON(403, gin.H{"error": "Unauthorized company access"})
		return
	}
	// One repeatable snapshot keeps total and rows consistent when another device creates a challan.
	tx, err := h.db.DB.BeginTx(c.Request.Context(), &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to fetch challans"})
		return
	}
	defer tx.Rollback()
	filter := ` FROM delivery_challans dc JOIN clients cl ON cl.id=dc.client_id AND cl.company_id=dc.company_id
 WHERE dc.company_id=$1 AND ($2='' OR dc.challan_number ILIKE '%' || $2 || '%' OR cl.name ILIKE '%' || $2 || '%')`
	var total int
	if err = tx.QueryRowContext(c.Request.Context(), `SELECT COUNT(*)`+filter, companyID, search).Scan(&total); err != nil {
		c.JSON(500, gin.H{"error": "Failed to fetch challans"})
		return
	}
	rows, err := tx.QueryContext(c.Request.Context(), `SELECT dc.id,dc.challan_number,TO_CHAR(dc.challan_date,'YYYY-MM-DD'),dc.status,
 cl.name,CASE WHEN EXISTS(SELECT 1 FROM invoices i WHERE i.id=dc.invoice_id AND i.company_id=dc.company_id AND i.client_id=dc.client_id) THEN dc.invoice_id END,dc.created_at`+filter+` ORDER BY dc.created_at DESC,dc.id DESC LIMIT $3 OFFSET $4`, companyID, search, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch challans"})
		return
	}
	defer rows.Close()

	type row struct {
		ID            int    `json:"id"`
		ChallanNumber string `json:"challan_number"`
		ChallanDate   string `json:"challan_date"`
		Status        string `json:"status"`
		ClientName    string `json:"client_name"`
		InvoiceID     *int64 `json:"invoice_id"`
		CreatedAt     string `json:"created_at"`
	}

	var challans []row
	for rows.Next() {
		var r row
		var invoiceID *int64
		var createdAt string
		if err := rows.Scan(&r.ID, &r.ChallanNumber, &r.ChallanDate, &r.Status, &r.ClientName, &invoiceID, &createdAt); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Scan error"})
			return
		}
		r.InvoiceID = invoiceID
		r.CreatedAt = createdAt
		challans = append(challans, r)
	}
	if challans == nil {
		challans = []row{}
	}
	if rows.Err() != nil {
		c.JSON(500, gin.H{"error": "Failed to fetch challans"})
		return
	}
	rows.Close()
	if err = tx.Commit(); err != nil {
		c.JSON(500, gin.H{"error": "Failed to fetch challans"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"challans": challans, "total": total, "limit": limit, "offset": offset, "has_more": offset+len(challans) < total})
}

// GET /challans/:id
func (h *ChallanHandler) GetChallanByID(c *gin.Context) {
	userID := c.GetInt("user_id")
	challanID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid id"})
		return
	}

	var id int
	var challanNumber, challanDate, status, notes, clientName string
	var clientID int
	var invoiceID *int64
	err = h.db.DB.QueryRowContext(c.Request.Context(),
		`SELECT dc.id, dc.challan_number, TO_CHAR(dc.challan_date,'YYYY-MM-DD'), dc.status,
		        COALESCE(dc.notes,''), dc.client_id, cl.name, CASE WHEN EXISTS(SELECT 1 FROM invoices i WHERE i.id=dc.invoice_id AND i.company_id=dc.company_id AND i.client_id=dc.client_id) THEN dc.invoice_id END
		 FROM delivery_challans dc
		 JOIN clients cl ON cl.id = dc.client_id AND cl.company_id = dc.company_id
		 WHERE dc.id = $1 AND dc.company_id IN (SELECT company_id FROM companies_for_user($2))`,
		challanID, userID,
	).Scan(&id, &challanNumber, &challanDate, &status, &notes, &clientID, &clientName, &invoiceID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Challan not found"})
		return
	}

	itemRows, err := h.db.DB.QueryContext(c.Request.Context(),
		`SELECT dci.id, dci.item_id, it.name, dci.qty, COALESCE(dci.description,'')
		 FROM delivery_challan_items dci
		 JOIN delivery_challans dc ON dc.id = dci.challan_id
         JOIN items it ON it.id = dci.item_id AND it.company_id = dc.company_id
		 WHERE dci.challan_id = $1 ORDER BY dci.id`,
		id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch items"})
		return
	}
	defer itemRows.Close()

	items := []gin.H{}
	for itemRows.Next() {
		var itemRowID, itemID, qty int
		var itemName, desc string
		if err := itemRows.Scan(&itemRowID, &itemID, &itemName, &qty, &desc); err != nil {
			c.JSON(500, gin.H{"error": "Failed to read challan items"})
			return
		}
		items = append(items, gin.H{"id": itemRowID, "item_id": itemID, "item_name": itemName, "qty": qty, "description": desc})
	}

	if itemRows.Err() != nil {
		c.JSON(500, gin.H{"error": "Failed to read challan items"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"id":             id,
		"challan_number": challanNumber,
		"challan_date":   challanDate,
		"status":         status,
		"notes":          notes,
		"client":         gin.H{"id": clientID, "name": clientName},
		"invoice_id":     invoiceID,
		"items":          items,
	})
}

// PATCH /challans/:id/dispatch
func (h *ChallanHandler) DispatchChallan(c *gin.Context) {
	userID := c.GetInt("user_id")
	challanID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid id"})
		return
	}

	result, err := h.db.DB.ExecContext(c.Request.Context(),
		`UPDATE delivery_challans SET status = 'dispatched', updated_at = NOW()
		 WHERE id = $1 AND status='draft' AND company_id IN (SELECT company_id FROM companies_for_user($2))
 AND EXISTS(SELECT 1 FROM clients cl WHERE cl.id=delivery_challans.client_id AND cl.company_id=delivery_challans.company_id)
 AND (invoice_id IS NULL OR EXISTS(SELECT 1 FROM invoices i WHERE i.id=delivery_challans.invoice_id AND i.company_id=delivery_challans.company_id AND i.client_id=delivery_challans.client_id))
 AND NOT EXISTS(SELECT 1 FROM delivery_challan_items di JOIN items it ON it.id=di.item_id WHERE di.challan_id=delivery_challans.id AND (it.company_id<>delivery_challans.company_id OR di.qty<=0))`,
		challanID, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to dispatch"})
		return
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "Challan is unavailable, already dispatched, or has invalid references"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "dispatched"})
}
