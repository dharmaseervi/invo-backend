package handlers

import (
	"database/sql"
	"fmt"
	database "invo-server/internal/db"
	"invo-server/internal/models"
	"invo-server/internal/services"
	utils "invo-server/internal/util"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

type InvoiceHandler struct {
	db            *database.Database
	LedgerService *services.LedgerService
}

func NewInvoiceHandler(db *database.Database) *InvoiceHandler {
	return &InvoiceHandler{db: db}
}

func insertInvoiceAddress(
	tx *sql.Tx,
	invoiceID int,
	addressType string,
	addr models.Address,
) error {

	_, err := tx.Exec(`
		INSERT INTO invoice_addresses (
			invoice_id, type,
			name, line1, line2, city, state,
			postal_code, country, phone, gst_number
		)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
	`,
		invoiceID,
		addressType,
		addr.Name,
		addr.Line1,
		addr.Line2,
		addr.City,
		addr.State,
		addr.PostalCode,
		addr.Country,
		addr.Phone,
		addr.GSTNumber,
	)

	return err
}

func fetchClientAddress(
	tx *sql.Tx,
	clientID int,
	addressType string, // billing | shipping
) (*models.Address, error) {

	var addr models.Address

	err := tx.QueryRow(`
		SELECT
			type,
			name,
			line1,
			line2,
			city,
			state,
			postal_code,
			country,
			phone,
			email,
			gst_number
		FROM client_addresses
		WHERE client_id = $1
		  AND type = $2
	`, clientID, addressType).Scan(
		&addr.AddressType,
		&addr.Name,
		&addr.Line1,
		&addr.Line2,
		&addr.City,
		&addr.State,
		&addr.PostalCode,
		&addr.Country,
		&addr.Phone,
		&addr.Email,
		&addr.GSTNumber,
	)

	if err != nil {
		return nil, err
	}

	return &addr, nil
}

// POST /api/v1/invoices
func (h *InvoiceHandler) CreateInvoice(c *gin.Context) {
	var req models.InvoiceRequestDTO
	var subtotal float64
	var taxTotal float64

	userID := c.GetInt("user_id")

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":  "Invalid input",
			"detail": err.Error(),
		})
		return
	}

	if len(req.Items) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invoice must contain at least one item",
		})
		return
	}

	// 1️⃣ Validate company ownership
	var companyExists bool
	err := h.db.DB.QueryRow(`
		SELECT EXISTS (
			SELECT 1 FROM companies
			WHERE id = $1 AND user_id = $2
		)
	`, req.CompanyID, userID).Scan(&companyExists)

	if err != nil || !companyExists {
		c.JSON(http.StatusForbidden, gin.H{"error": "Unauthorized company access"})
		return
	}

	// 2️⃣ Validate client
	var clientExists bool
	err = h.db.DB.QueryRow(`
		SELECT EXISTS (
			SELECT 1 FROM clients
			WHERE id = $1 AND user_id = $2 AND company_id = $3
		)
	`, req.ClientID, userID, req.CompanyID).Scan(&clientExists)

	if err != nil || !clientExists {
		c.JSON(http.StatusForbidden, gin.H{"error": "Invalid or unauthorized client"})
		return
	}

	// 3️⃣ Validate items
	for _, item := range req.Items {
		var itemExists bool
		err = h.db.DB.QueryRow(`
			SELECT EXISTS (
				SELECT 1 FROM items
				WHERE id = $1 AND user_id = $2 AND company_id = $3
			)
		`, item.ItemID, userID, req.CompanyID).Scan(&itemExists)

		if err != nil || !itemExists {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Invalid or unauthorized item",
				"item_id": item.ItemID,
			})
			return
		}
	}

	for _, item := range req.Items {
		lineBase := item.Rate * float64(item.Qty)
		lineAfterDiscount := lineBase - item.Discount
		lineTax := lineAfterDiscount * (item.TaxRate / 100)

		subtotal += lineAfterDiscount
		taxTotal += lineTax

		// insert invoice_items with lineTotal
	}

	grandTotal := subtotal + taxTotal

	// 4️⃣ Parse dates
	invDate, err := time.Parse("2006-01-02", req.InvoiceDate)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid invoice_date (YYYY-MM-DD)"})
		return
	}

	dueDate, err := time.Parse("2006-01-02", req.DueDate)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid due_date (YYYY-MM-DD)"})
		return
	}

	// 5️⃣ Begin transaction
	tx, err := h.db.DB.Begin()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to start transaction"})
		return
	}

	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	// 6️⃣ Generate invoice number (FY based)
	fy := utils.FinancialYear(invDate)

	var nextNumber int
	err = tx.QueryRow(`
		INSERT INTO invoice_counters (company_id, financial_year)
		VALUES ($1, $2)
		ON CONFLICT (company_id, financial_year)
		DO UPDATE SET next_number = invoice_counters.next_number + 1
		RETURNING next_number
	`, req.CompanyID, fy).Scan(&nextNumber)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate invoice number"})
		return
	}

	invoiceNumber := fmt.Sprintf("INV/%s/%04d", fy, nextNumber)

	// 7️⃣ Insert invoice
	var invoiceID int
	err = tx.QueryRow(`
		INSERT INTO invoices (
			company_id,
			user_id,
			client_id,
			invoice_number,
			invoice_date,
			due_date,
			subtotal,
			tax,
			total,
			status,
			paid_amount,
			remaining_amount
		)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,'draft',0,$9)
		RETURNING id
	`,
		req.CompanyID,
		userID,
		req.ClientID,
		invoiceNumber,
		invDate,
		dueDate,
		subtotal,
		taxTotal,
		grandTotal,
	).Scan(&invoiceID)

	if err != nil {
		fmt.Println("SQL ERROR:", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":  "Failed to create invoice",
			"detail": err.Error(),
		})
		return
	}

	// err = h.LedgerService.AddEntryTx(
	// 	tx,
	// 	int64(req.CompanyID),
	// 	int64(req.ClientID),
	// 	"INVOICE",
	// 	int64(invoiceID),
	// 	grandTotal,
	// 	0,
	// 	"Invoice "+invoiceNumber,
	// )
	// if err != nil {
	// 	fmt.Println("SQL ERROR:", err)
	// 	c.JSON(http.StatusInternalServerError, gin.H{
	// 		"error": "Failed to create ledger entry",
	// 	})
	// 	return
	// }

	// 8️⃣ Insert invoice items
	for _, item := range req.Items {
		lineTotal := (item.Rate * float64(item.Qty)) - item.Discount
		lineTotal += lineTotal * (item.TaxRate / 100)

		_, err = tx.Exec(`
			INSERT INTO invoice_items
				(invoice_id, item_id, qty, rate, discount, tax_rate, total)
			VALUES ($1,$2,$3,$4,$5,$6,$7)
		`,
			invoiceID,
			item.ItemID,
			item.Qty,
			item.Rate,
			item.Discount,
			item.TaxRate,
			lineTotal,
		)

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":  "Failed to create invoice items",
				"detail": err.Error(),
			})
			return
		}
	}
	// 1️⃣1️⃣ Fetch client addresses (snapshot)
	billingAddr, err := fetchClientAddress(tx, req.ClientID, "billing")
	if err != nil {
		fmt.Println("SQL ERROR:", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Client billing address is required",
		})
		return
	}

	shippingAddr, _ := fetchClientAddress(tx, req.ClientID, "shipping")

	// 1️⃣2️⃣ Insert invoice address snapshot
	if err := insertInvoiceAddress(tx, invoiceID, "billing", *billingAddr); err != nil {
		fmt.Println("SQL ERROR:", err)
		c.JSON(http.StatusInternalServerError, gin.H{

			"error": "Failed to save invoice billing address",
		})
		return
	}

	if shippingAddr != nil {
		if err := insertInvoiceAddress(tx, invoiceID, "shipping", *shippingAddr); err != nil {
			fmt.Println("SQL ERROR:", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Failed to save invoice shipping address",
			})
			return
		}
	}

	// 1️⃣3️⃣ Commit transaction
	if err = tx.Commit(); err != nil {
		log.Printf("Failed to commit transaction: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to commit transaction",
		})
		return
	}

	committed = true

	// 🔟 Response
	c.JSON(http.StatusCreated, gin.H{
		"message":        "Invoice created successfully",
		"invoice_id":     invoiceID,
		"invoice_number": invoiceNumber,
		"financial_year": fy,
	})
}

func (h *InvoiceHandler) UpdateInvoice(c *gin.Context) {
	invoiceID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid invoice id"})
		return
	}

	userID := c.GetInt("user_id")

	var req models.UpdateInvoiceRequestDTO
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":  "Invalid input",
			"detail": err.Error(),
		})
		return
	}

	if len(req.Items) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invoice must contain at least one item",
		})
		return
	}

	// 1️⃣ Fetch invoice & validate ownership + status
	var (
		companyID int
		status    string
	)

	err = h.db.DB.QueryRow(`
		SELECT company_id, status
		FROM invoices
		WHERE id = $1 AND user_id = $2
	`, invoiceID, userID).Scan(&companyID, &status)

	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Invoice not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch invoice"})
		return
	}

	if status != "draft" {
		c.JSON(http.StatusConflict, gin.H{
			"error": "Only draft invoices can be edited",
		})
		return
	}

	// 2️⃣ Validate client
	var clientExists bool
	err = h.db.DB.QueryRow(`
		SELECT EXISTS (
			SELECT 1 FROM clients
			WHERE id = $1 AND user_id = $2 AND company_id = $3
		)
	`, req.ClientID, userID, companyID).Scan(&clientExists)

	if err != nil || !clientExists {
		c.JSON(http.StatusForbidden, gin.H{"error": "Invalid or unauthorized client"})
		return
	}

	// 3️⃣ Validate items
	for _, item := range req.Items {
		var itemExists bool
		err = h.db.DB.QueryRow(`
			SELECT EXISTS (
				SELECT 1 FROM items
				WHERE id = $1 AND user_id = $2 AND company_id = $3
			)
		`, item.ItemID, userID, companyID).Scan(&itemExists)

		if err != nil || !itemExists {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Invalid or unauthorized item",
				"item_id": item.ItemID,
			})
			return
		}
	}

	// 4️⃣ Calculate totals
	var subtotal, taxTotal float64

	for _, item := range req.Items {
		lineBase := item.Rate * float64(item.Qty)
		lineAfterDiscount := lineBase - item.Discount
		lineTax := lineAfterDiscount * (item.TaxRate / 100)

		subtotal += lineAfterDiscount
		taxTotal += lineTax
	}

	total := subtotal + taxTotal

	// 5️⃣ Parse dates
	invDate, err := time.Parse("2006-01-02", req.InvoiceDate)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid invoice_date"})
		return
	}

	dueDate, err := time.Parse("2006-01-02", req.DueDate)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid due_date"})
		return
	}

	// 6️⃣ Transaction
	tx, err := h.db.DB.Begin()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to start transaction"})
		return
	}

	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	// 7️⃣ Update invoice header
	_, err = tx.Exec(`
		UPDATE invoices
		SET
			client_id = $1,
			invoice_date = $2,
			due_date = $3,
			subtotal = $4,
			tax = $5,
			total = $6,
			remaining_amount = $6,
			updated_at = NOW()
		WHERE id = $7
	`,
		req.ClientID,
		invDate,
		dueDate,
		subtotal,
		taxTotal,
		total,
		invoiceID,
	)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update invoice"})
		return
	}

	// 8️⃣ Replace invoice items
	_, err = tx.Exec(`DELETE FROM invoice_items WHERE invoice_id = $1`, invoiceID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to clear invoice items"})
		return
	}

	for _, item := range req.Items {
		lineTotal := (item.Rate * float64(item.Qty)) - item.Discount
		lineTotal += lineTotal * (item.TaxRate / 100)

		_, err = tx.Exec(`
			INSERT INTO invoice_items
				(invoice_id, item_id, qty, rate, discount, tax_rate, total)
			VALUES ($1,$2,$3,$4,$5,$6,$7)
		`,
			invoiceID,
			item.ItemID,
			item.Qty,
			item.Rate,
			item.Discount,
			item.TaxRate,
			lineTotal,
		)

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to insert invoice items"})
			return
		}
	}

	// 9️⃣ Commit
	if err = tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to commit transaction"})
		return
	}
	committed = true

	// 🔟 Response
	c.JSON(http.StatusOK, gin.H{
		"message":    "Invoice updated successfully",
		"invoice_id": invoiceID,
	})
}

// GET /api/v1/invoices
func (h *InvoiceHandler) GetInvoices(c *gin.Context) {
	userID := c.GetInt("user_id")

	companyIDStr := c.Query("company_id")
	clientIDStr := c.Query("client_id")

	limitStr := c.DefaultQuery("limit", "10")
	offsetStr := c.DefaultQuery("offset", "0")

	limit, _ := strconv.Atoi(limitStr)
	if limit <= 0 {
		limit = 10
	}

	offset, _ := strconv.Atoi(offsetStr)
	if offset < 0 {
		offset = 0
	}

	query := `
		SELECT
			id,
			company_id,
			client_id,
			invoice_number,
			invoice_date,
			due_date,
			subtotal,
			tax,
			total,
			paid_amount,
			remaining_amount,
			status,
			created_at,
			GREATEST(0, CURRENT_DATE - due_date) AS days_overdue,
			CURRENT_DATE > due_date AND status != 'paid' AS is_overdue
		FROM invoices
		WHERE user_id = $1
	`

	args := []interface{}{userID}
	argPos := 2

	if companyIDStr != "" {
		if companyID, err := strconv.Atoi(companyIDStr); err == nil {
			query += ` AND company_id = $` + strconv.Itoa(argPos)
			args = append(args, companyID)
			argPos++
		}
	}

	if clientIDStr != "" {
		if clientID, err := strconv.Atoi(clientIDStr); err == nil {
			query += ` AND client_id = $` + strconv.Itoa(argPos)
			args = append(args, clientID)
			argPos++
		}
	}

	query += `
		ORDER BY invoice_date DESC
		LIMIT $` + strconv.Itoa(argPos) +
		` OFFSET $` + strconv.Itoa(argPos+1)

	args = append(args, limit, offset)

	rows, err := h.db.DB.Query(query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to fetch invoices",
		})
		return
	}
	defer rows.Close()

	var invoices []gin.H

	for rows.Next() {
		var (
			id, companyID, clientID int
			invoiceNumber, status   string
			invoiceDate, dueDate    time.Time
			createdAt               time.Time
			subtotal, tax, total    float64
			paidAmount, remaining   float64
			daysOverdue             int
			isOverdue               bool
		)

		if err := rows.Scan(
			&id,
			&companyID,
			&clientID,
			&invoiceNumber,
			&invoiceDate,
			&dueDate,
			&subtotal,
			&tax,
			&total,
			&paidAmount,
			&remaining,
			&status,
			&createdAt,
			&daysOverdue,
			&isOverdue,
		); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Failed to scan invoice",
			})
			return
		}

		invoices = append(invoices, gin.H{
			"id":               id,
			"company_id":       companyID,
			"client_id":        clientID,
			"invoice_number":   invoiceNumber,
			"invoice_date":     invoiceDate.Format("2006-01-02"),
			"due_date":         dueDate.Format("2006-01-02"),
			"subtotal":         subtotal,
			"tax":              tax,
			"total":            total,
			"paid_amount":      paidAmount,
			"remaining_amount": remaining,
			"status":           status,
			"is_overdue":       isOverdue,
			"days_overdue":     daysOverdue,
			"created_at":       createdAt,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"data":   invoices,
		"limit":  limit,
		"offset": offset,
	})
}

// GET /api/v1/invoices/:id
func (h *InvoiceHandler) GetInvoiceByID(c *gin.Context) {
	userID := c.GetInt("user_id")
	invoiceID := c.Param("id")

	var (
		id, companyID, clientID int
		invoiceNumber, status   string
		clientName              string
		invoiceDate, dueDate    time.Time
		createdAt               time.Time
		subtotal, tax, total    float64
		paidAmount, remaining   float64
		daysOverdue             int
		isOverdue               bool
	)

	err := h.db.DB.QueryRow(`
		SELECT
			i.id,
			i.company_id,
			i.client_id,
			i.invoice_number,
			i.invoice_date,
			i.due_date,
			i.subtotal,
			i.tax,
			i.total,
			i.paid_amount,
			i.remaining_amount,
			i.status,
			i.created_at,
			c.name,
			GREATEST(0, CURRENT_DATE - i.due_date) AS days_overdue,
			CURRENT_DATE > i.due_date AND i.status != 'paid' AS is_overdue
		FROM invoices i
		JOIN clients c ON c.id = i.client_id
		WHERE i.id = $1 AND i.user_id = $2
	`, invoiceID, userID).Scan(
		&id,
		&companyID,
		&clientID,
		&invoiceNumber,
		&invoiceDate,
		&dueDate,
		&subtotal,
		&tax,
		&total,
		&paidAmount,
		&remaining,
		&status,
		&createdAt,
		&clientName,
		&daysOverdue,
		&isOverdue,
	)

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Invoice not found",
		})
		return
	}

	// Fetch invoice items
	rows, err := h.db.DB.Query(`
		SELECT
			ii.id,
			ii.item_id,
			ii.qty,
			ii.rate,
			ii.discount,
			ii.tax_rate,
			ii.total
		FROM invoice_items ii
		WHERE ii.invoice_id = $1
		ORDER BY ii.id
	`, id)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to fetch invoice items",
		})
		return
	}
	defer rows.Close()

	var items []gin.H
	for rows.Next() {
		var (
			itemRowID, itemID, qty             int
			rate, discount, taxRate, lineTotal float64
		)

		if err := rows.Scan(
			&itemRowID,
			&itemID,
			&qty,
			&rate,
			&discount,
			&taxRate,
			&lineTotal,
		); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Failed to scan invoice item",
			})
			return
		}

		items = append(items, gin.H{
			"id":       itemRowID,
			"item_id":  itemID,
			"qty":      qty,
			"rate":     rate,
			"discount": discount,
			"tax_rate": taxRate,
			"total":    lineTotal,
		})
	}

	// Final response
	c.JSON(http.StatusOK, gin.H{
		"id":               id,
		"invoice_number":   invoiceNumber,
		"status":           status,
		"invoice_date":     invoiceDate.Format("2006-01-02"),
		"due_date":         dueDate.Format("2006-01-02"),
		"subtotal":         subtotal,
		"tax":              tax,
		"total":            total,
		"paid_amount":      paidAmount,
		"remaining_amount": remaining,
		"is_overdue":       isOverdue,
		"days_overdue":     daysOverdue,
		"client": gin.H{
			"id":   clientID,
			"name": clientName,
		},
		"items":      items,
		"created_at": createdAt,
	})
}

// GET /api/v1/invoices/number-preview
func (h *InvoiceHandler) GetInvoiceNumberPreview(c *gin.Context) {
	userID := c.GetInt("user_id")
	companyID := c.Query("company_id")

	if companyID == "" {
		c.JSON(400, gin.H{"error": "company_id is required"})
		return
	}

	// Verify company ownership
	var exists bool
	err := h.db.DB.QueryRow(`
		SELECT EXISTS (
			SELECT 1 FROM companies
			WHERE id = $1 AND user_id = $2
		)
	`, companyID, userID).Scan(&exists)

	if err != nil || !exists {
		c.JSON(403, gin.H{"error": "Unauthorized company"})
		return
	}

	fy := utils.FinancialYear(time.Now()) // "2024-25"

	var next int
	err = h.db.DB.QueryRow(`
		SELECT COALESCE(next_number, 0) + 1
        FROM invoice_counters
        WHERE company_id = $1 AND financial_year = $2 
	`, companyID, fy).Scan(&next)

	if err != nil {
		next = 1
	}

	preview := fmt.Sprintf("INV/%s/%06d", fy, next)

	c.JSON(200, gin.H{
		"preview": preview,
	})
}

func (h *InvoiceHandler) GetUnpaidInvoices(c *gin.Context) {
	clientID, _ := strconv.ParseInt(c.Param("clientId"), 10, 64)

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

	rows, err := h.db.DB.Query(`
		SELECT id, invoice_number, remaining_amount, invoice_date
		FROM invoices
		WHERE company_id = $1
		  AND client_id = $2
		  AND remaining_amount > 0
		ORDER BY invoice_date ASC
	`, companyID, clientID)

	if err != nil {
		fmt.Println("SQL ERROR:", err)
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var invoices []models.InvoiceSummary
	for rows.Next() {
		var inv models.InvoiceSummary
		rows.Scan(
			&inv.ID,
			&inv.InvoiceNumber,
			&inv.RemainingAmount,
			&inv.InvoiceDate,
		)
		invoices = append(invoices, inv)
	}
	fmt.Println("Unpaid invoices:", invoices)

	c.JSON(200, gin.H{"data": invoices})
}

// GET /api/v1/clients/:clientId/invoices
func (h *InvoiceHandler) GetInvoicesByClientID(c *gin.Context) {

	clientID, err := strconv.ParseInt(c.Param("clientId"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid client id"})
		return
	}

	userID := c.GetInt("user_id")

	// 🔐 Verify client ownership
	var companyID int64
	err = h.db.DB.QueryRow(`
		SELECT company_id
		FROM clients
		WHERE id = $1 AND user_id = $2
	`, clientID, userID).Scan(&companyID)

	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "unauthorized client"})
		return
	}

	rows, err := h.db.DB.Query(`
		SELECT
			id,
			invoice_number,
			invoice_date,
			due_date,
			subtotal,
			tax,
			total,
			paid_amount,
			remaining_amount,
			status,
			created_at
		FROM invoices
		WHERE client_id = $1
		ORDER BY invoice_date DESC
	`, clientID)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var invoices []models.Invoice

	for rows.Next() {
		var inv models.Invoice
		err := rows.Scan(
			&inv.ID,
			&inv.InvoiceNumber,
			&inv.InvoiceDate,
			&inv.DueDate,
			&inv.Subtotal,
			&inv.Tax,
			&inv.Total,
			&inv.PaidAmount,
			&inv.RemainingAmount,
			&inv.Status,
			&inv.CreatedAt,
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		invoices = append(invoices, inv)
	}

	c.JSON(http.StatusOK, gin.H{
		"data": invoices,
	})
}

func (h *InvoiceHandler) IssueInvoice(c *gin.Context) {
	invoiceID, _ := strconv.Atoi(c.Param("id"))
	userID := c.GetInt("user_id")

	tx, err := h.db.DB.Begin()
	if err != nil {
		c.JSON(500, gin.H{"error": "failed to start transaction"})
		return
	}
	defer tx.Rollback()

	var (
		status    string
		total     float64
		clientID  int64
		companyID int64
		number    string
	)

	err = tx.QueryRow(`
        SELECT status, total, client_id, company_id, invoice_number
        FROM invoices
        WHERE id = $1 AND user_id = $2
    `, invoiceID, userID).Scan(
		&status, &total, &clientID, &companyID, &number,
	)

	if err != nil {
		c.JSON(404, gin.H{"error": "invoice not found"})
		return
	}

	if status != "draft" {
		c.JSON(400, gin.H{"error": "invoice already issued"})
		return
	}

	// 1️⃣ Update invoice
	_, err = tx.Exec(`
        UPDATE invoices
        SET status = 'issued',
            remaining_amount = total
        WHERE id = $1
    `, invoiceID)
	if err != nil {
		c.JSON(500, gin.H{"error": "failed to update invoice"})
		return
	}

	// 2️⃣ Ledger entry (THIS is the correct place)
	err = h.LedgerService.AddEntryTx(
		tx,
		companyID,
		clientID,
		"INVOICE",
		int64(invoiceID),
		total,
		0,
		"Invoice "+number,
	)
	if err != nil {
		c.JSON(500, gin.H{"error": "failed to create ledger entry"})
		return
	}

	tx.Commit()

	c.JSON(200, gin.H{"message": "Invoice issued successfully"})
}
