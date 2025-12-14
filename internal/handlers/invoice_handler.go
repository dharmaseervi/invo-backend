package handlers

import (
	"fmt"
	database "invo-server/internal/db"
	"invo-server/internal/models"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

type InvoiceHandler struct {
	db *database.Database
}

func NewInvoiceHandler(db *database.Database) *InvoiceHandler {
	return &InvoiceHandler{db: db}
}

// POST /api/v1/invoices
func (h *InvoiceHandler) CreateInvoice(c *gin.Context) {
	var req models.InvoiceRequestDTO
	userID := c.GetInt("user_id")
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid input", "detail": err.Error()})
		return
	}

	// 1) Validate company belongs to user
	var companyExists bool
	err := h.db.DB.QueryRow(`
		SELECT EXISTS(
			SELECT 1 FROM companies
			WHERE id = $1 AND user_id = $2
		)
	`, req.CompanyID, userID).Scan(&companyExists)
	if err != nil || !companyExists {
		c.JSON(http.StatusForbidden, gin.H{"error": "Unauthorized company access"})
		return
	}

	// 2) Validate client belongs to this user + company
	var clientExists bool
	err = h.db.DB.QueryRow(`
		SELECT EXISTS(
			SELECT 1 FROM clients
			WHERE id = $1 AND user_id = $2 AND company_id = $3
		)
	`, req.ClientID, userID, req.CompanyID).Scan(&clientExists)
	if err != nil || !clientExists {
		c.JSON(http.StatusForbidden, gin.H{"error": "Invalid or unauthorized client"})
		return
	}

	// 3) Optional: validate all items belong to this user + company
	for _, itemReq := range req.Items {
		var itemExists bool
		err = h.db.DB.QueryRow(`
			SELECT EXISTS(
				SELECT 1 FROM items
				WHERE id = $1 AND user_id = $2 AND company_id = $3
			)
		`, itemReq.ItemID, userID, req.CompanyID).Scan(&itemExists)
		if err != nil || !itemExists {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid or unauthorized item", "item_id": itemReq.ItemID})
			return
		}
	}

	// 4) Parse dates
	invDate, err := time.Parse("2006-01-02", req.InvoiceDate)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid invoice_date format, expected YYYY-MM-DD"})
		return
	}
	dueDate, err := time.Parse("2006-01-02", req.DueDate)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid due_date format, expected YYYY-MM-DD"})
		return
	}

	if req.InvoiceNumber == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invoice_number is required",
		})
		return
	}

	tx, err := h.db.DB.Begin()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to start transaction"})
		return
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	// 5) Insert invoice
	var invoiceID int
	err = tx.QueryRow(`
	INSERT INTO invoices 
		(company_id, user_id, client_id, invoice_number, invoice_date, due_date, subtotal, tax, total)
	VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
	RETURNING id
`,
		req.CompanyID,
		userID,
		req.ClientID,
		req.InvoiceNumber,
		invDate,
		dueDate,
		req.Subtotal,
		req.Tax,
		req.Total,
	).Scan(&invoiceID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create invoice", "detail": err.Error()})
		return
	}

	// 6) Insert invoice items
	for _, itemReq := range req.Items {
		lineTotal := (itemReq.Rate * float64(itemReq.Qty)) - itemReq.Discount
		lineTotal += lineTotal * (itemReq.TaxRate / 100.0)
		_, err = tx.Exec(`
			INSERT INTO invoice_items
				(invoice_id, item_id, qty, rate, discount, tax_rate, total)
			VALUES ($1,$2,$3,$4,$5,$6,$7)
		`,
			invoiceID,
			itemReq.ItemID,
			itemReq.Qty,
			itemReq.Rate,
			itemReq.Discount,
			itemReq.TaxRate,
			lineTotal,
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create invoice items", "detail": err.Error()})
			return
		}
	}

	if err = tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to commit invoice", "detail": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message":    "Invoice created successfully",
		"invoice_id": invoiceID,
	})
}

// GET /api/v1/invoices
// Query params: company_id, client_id, limit, offset
func (h *InvoiceHandler) GetInvoices(c *gin.Context) {
	userID := c.GetInt("user_id")
	companyIDStr := c.Query("company_id")
	clientIDStr := c.Query("client_id")
	limit := c.DefaultQuery("limit", "10")
	offset := c.DefaultQuery("offset", "0")

	// Parse limit and offset
	limitInt, err := strconv.Atoi(limit)
	if err != nil || limitInt <= 0 {
		limitInt = 10
	}
	offsetInt, err := strconv.Atoi(offset)
	if err != nil || offsetInt < 0 {
		offsetInt = 0
	}

	fmt.Printf("GetInvoices called - userID: %d, company_id: %s, limit: %d, offset: %d\n",
		userID, companyIDStr, limitInt, offsetInt)

	// Build query dynamically
	query := `
		SELECT id, company_id, client_id, invoice_number, invoice_date, 
		       due_date, subtotal, tax, total, created_at
		FROM invoices
		WHERE user_id = $1
	`
	args := []interface{}{userID}
	argCount := 2

	// Add optional filters
	if companyIDStr != "" {
		companyID, err := strconv.Atoi(companyIDStr)
		if err == nil {
			query += ` AND company_id = $` + strconv.Itoa(argCount)
			args = append(args, companyID)
			argCount++
		}
	}

	if clientIDStr != "" {
		clientID, err := strconv.Atoi(clientIDStr)
		if err == nil {
			query += ` AND client_id = $` + strconv.Itoa(argCount)
			args = append(args, clientID)
			argCount++
		}
	}

	query += ` ORDER BY created_at DESC LIMIT $` + strconv.Itoa(argCount) + ` OFFSET $` + strconv.Itoa(argCount+1)
	args = append(args, limitInt, offsetInt)

	fmt.Printf("Executing query: %s\nWith args: %v\n", query, args)

	rows, err := h.db.DB.Query(query, args...)
	if err != nil {
		fmt.Printf("Query error: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch invoices", "detail": err.Error()})
		return
	}
	defer rows.Close()

	var invoices []gin.H
	for rows.Next() {
		var id, companyID, clientID int
		var invoiceNumber string
		var invoiceDate, dueDate time.Time
		var subtotal, tax, total float64
		var createdAt time.Time

		err := rows.Scan(&id, &companyID, &clientID, &invoiceNumber, &invoiceDate,
			&dueDate, &subtotal, &tax, &total, &createdAt)
		if err != nil {
			fmt.Printf("Scan error: %v\n", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to scan invoice"})
			return
		}

		invoices = append(invoices, gin.H{
			"id":             id,
			"company_id":     companyID,
			"client_id":      clientID,
			"invoice_number": invoiceNumber,
			"invoice_date":   invoiceDate.Format("2006-01-02"),
			"due_date":       dueDate.Format("2006-01-02"),
			"subtotal":       subtotal,
			"tax":            tax,
			"total":          total,
			"created_at":     createdAt,
		})

		fmt.Printf("✅ Fetched invoice ID: %d, Number: %s\n", id, invoiceNumber)
	}

	if err = rows.Err(); err != nil {
		fmt.Printf("Row iteration error: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error iterating invoices"})
		return
	}

	fmt.Printf("Total invoices fetched: %d\n", len(invoices))
	c.JSON(http.StatusOK, gin.H{
		"data":   invoices,
		"limit":  limitInt,
		"offset": offsetInt,
	})
}

// GET /api/v1/invoices/:id
// GET /api/v1/invoices/:id
func (h *InvoiceHandler) GetInvoiceByID(c *gin.Context) {
	userID := c.GetInt("user_id")
	invoiceID := c.Param("id")

	fmt.Printf("GetInvoiceByID called - invoiceID: %s, userID: %d\n", invoiceID, userID)

	// Fetch invoice details
	var id, companyID, clientID int
	var invoiceNumber string
	var invoiceDate, dueDate time.Time
	var subtotal, tax, total float64
	var createdAt time.Time
	var clientName string

	err := h.db.DB.QueryRow(`
		SELECT i.id, i.company_id, i.client_id, i.invoice_number, i.invoice_date,
		       i.due_date, i.subtotal, i.tax, i.total, i.created_at, c.name
		FROM invoices i
		JOIN clients c ON i.client_id = c.id
		WHERE i.id = $1 AND i.user_id = $2
	`, invoiceID, userID).Scan(
		&id, &companyID, &clientID, &invoiceNumber, &invoiceDate,
		&dueDate, &subtotal, &tax, &total, &createdAt, &clientName,
	)

	if err != nil {
		fmt.Printf("Invoice not found: %v\n", err)
		c.JSON(http.StatusNotFound, gin.H{"error": "Invoice not found"})
		return
	}

	// Fetch invoice items
	itemsRows, err := h.db.DB.Query(`
		SELECT ii.id, ii.item_id, ii.qty, ii.rate, ii.discount, ii.tax_rate, ii.total
		FROM invoice_items ii
		WHERE ii.invoice_id = $1
	`, id)
	if err != nil {
		fmt.Printf("Failed to fetch items: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch invoice items"})
		return
	}
	defer itemsRows.Close()

	var items []gin.H
	for itemsRows.Next() {
		var itemRowID, itemID, qty int
		var rate, discount, taxRate, total float64

		err := itemsRows.Scan(&itemRowID, &itemID, &qty, &rate, &discount, &taxRate, &total)
		if err != nil {
			fmt.Printf("Item scan error: %v\n", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to scan invoice item"})
			return
		}

		items = append(items, gin.H{
			"id":       itemRowID,
			"item_id":  itemID,
			"qty":      qty,
			"rate":     rate,
			"discount": discount,
			"tax_rate": taxRate,
			"total":    total,
		})
	}

	// Return simplified response
	c.JSON(http.StatusOK, gin.H{
		"id":             id,
		"invoice_number": invoiceNumber,
		"client_name":    clientName,
		"invoice_date":   invoiceDate.Format("2006-01-02"),
		"due_date":       dueDate.Format("2006-01-02"),
		"subtotal":       subtotal,
		"tax":            tax,
		"total":          total,
		"items":          items,
	})
}
