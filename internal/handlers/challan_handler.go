package handlers

import (
	"fmt"
	"net/http"
	"strconv"

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

	// Get next challan number
	var nextNum int
	err = h.db.DB.QueryRowContext(c.Request.Context(),
		`SELECT nextval('delivery_challan_seq')`).Scan(&nextNum)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate challan number"})
		return
	}
	challanNumber := fmt.Sprintf("DC/%04d", nextNum)

	tx, err := h.db.DB.BeginTx(c.Request.Context(), nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to start transaction"})
		return
	}
	defer tx.Rollback()

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
			`INSERT INTO delivery_challan_items (challan_id, item_id, qty, description) VALUES ($1,$2,$3,$4)`,
			challanID, item.ItemID, item.Qty, item.Description)
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

	rows, err := h.db.DB.QueryContext(c.Request.Context(),
		`SELECT dc.id, dc.challan_number, TO_CHAR(dc.challan_date,'YYYY-MM-DD'), dc.status,
		        cl.name, dc.invoice_id, dc.created_at
		 FROM delivery_challans dc
		 JOIN clients cl ON cl.id = dc.client_id
		 WHERE dc.company_id = $1
		   AND dc.company_id IN (SELECT company_id FROM companies_for_user($2))
		 ORDER BY dc.created_at DESC`,
		companyID, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch challans"})
		return
	}
	defer rows.Close()

	type row struct {
		ID            int     `json:"id"`
		ChallanNumber string  `json:"challan_number"`
		ChallanDate   string  `json:"challan_date"`
		Status        string  `json:"status"`
		ClientName    string  `json:"client_name"`
		InvoiceID     *int64  `json:"invoice_id"`
		CreatedAt     string  `json:"created_at"`
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
	c.JSON(http.StatusOK, gin.H{"challans": challans})
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
		        COALESCE(dc.notes,''), dc.client_id, cl.name, dc.invoice_id
		 FROM delivery_challans dc
		 JOIN clients cl ON cl.id = dc.client_id
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
		 JOIN items it ON it.id = dci.item_id
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
			continue
		}
		items = append(items, gin.H{"id": itemRowID, "item_id": itemID, "item_name": itemName, "qty": qty, "description": desc})
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
		 WHERE id = $1 AND company_id IN (SELECT company_id FROM companies_for_user($2))`,
		challanID, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to dispatch"})
		return
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Challan not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "dispatched"})
}
