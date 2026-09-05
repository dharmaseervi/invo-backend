package handlers

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"time"

	database "invo-server/internal/db"
	"invo-server/internal/models"
	"invo-server/internal/pdf"
	"invo-server/internal/services"
	utils "invo-server/internal/util"

	"github.com/gin-gonic/gin"
)

type EstimateHandler struct {
	db *database.Database
}

func NewEstimateHandler(db *database.Database) *EstimateHandler {
	return &EstimateHandler{db: db}
}

// POST /api/v1/estimates
func (h *EstimateHandler) CreateEstimate(c *gin.Context) {
	var req models.EstimateRequestDTO
	userID := c.GetInt("user_id")

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid input", "detail": err.Error()})
		return
	}
	if len(req.Items) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Estimate must contain at least one item"})
		return
	}

	owned, err := companyBelongsToUser(h.db.DB, int64(req.CompanyID), userID)
	if err != nil || !owned {
		c.JSON(http.StatusForbidden, gin.H{"error": "Unauthorized company access"})
		return
	}

	var clientExists bool
	err = h.db.DB.QueryRow(`
		SELECT EXISTS (SELECT 1 FROM clients WHERE id = $1 AND user_id = $2 AND company_id = $3)
	`, req.ClientID, userID, req.CompanyID).Scan(&clientExists)
	if err != nil || !clientExists {
		c.JSON(http.StatusForbidden, gin.H{"error": "Invalid or unauthorized client"})
		return
	}

	var subtotal, taxTotal float64
	for _, item := range req.Items {
		lineBase := item.Rate * float64(item.Qty)
		lineAfterDiscount := lineBase - item.Discount
		subtotal += lineAfterDiscount
		taxTotal += lineAfterDiscount * (item.TaxRate / 100)
	}

	preDiscountTotal := subtotal + taxTotal
	discount := req.Discount
	if discount < 0 {
		discount = 0
	}
	if discount > preDiscountTotal {
		discount = preDiscountTotal
	}
	total := preDiscountTotal - discount

	estimateDate, err := time.Parse("2006-01-02", req.EstimateDate)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid estimate_date (YYYY-MM-DD)"})
		return
	}
	var expiryDate *time.Time
	if req.ExpiryDate != nil && *req.ExpiryDate != "" {
		parsed, err := time.Parse("2006-01-02", *req.ExpiryDate)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid expiry_date (YYYY-MM-DD)"})
			return
		}
		expiryDate = &parsed
	}

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

	fy := utils.FinancialYear(estimateDate)
	var nextNumber int
	err = tx.QueryRow(`
		INSERT INTO estimate_counters (company_id, financial_year)
		VALUES ($1, $2)
		ON CONFLICT (company_id, financial_year)
		DO UPDATE SET next_number = estimate_counters.next_number + 1
		RETURNING next_number
	`, req.CompanyID, fy).Scan(&nextNumber)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate estimate number"})
		return
	}
	estimateNumber := fmt.Sprintf("EST/%s/%04d", fy, nextNumber)

	var estimateID int
	err = tx.QueryRow(`
		INSERT INTO estimates (
			company_id, user_id, client_id, estimate_number,
			estimate_date, expiry_date, subtotal, tax, discount, total, status
		)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,'draft')
		RETURNING id
	`,
		req.CompanyID, userID, req.ClientID, estimateNumber,
		estimateDate, expiryDate, subtotal, taxTotal, discount, total,
	).Scan(&estimateID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create estimate", "detail": err.Error()})
		return
	}

	for _, item := range req.Items {
		lineTotal := (item.Rate * float64(item.Qty)) - item.Discount
		lineTotal += lineTotal * (item.TaxRate / 100)

		_, err = tx.Exec(`
			INSERT INTO estimate_items (estimate_id, item_id, qty, rate, discount, tax_rate, total)
			VALUES ($1,$2,$3,$4,$5,$6,$7)
		`, estimateID, item.ItemID, item.Qty, item.Rate, item.Discount, item.TaxRate, lineTotal)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to add estimate items"})
			return
		}
	}

	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to commit transaction"})
		return
	}
	committed = true

	c.JSON(http.StatusCreated, models.CreateEstimateResponse{
		EstimateID:     estimateID,
		EstimateNumber: estimateNumber,
		FinancialYear:  fy,
	})
}

// GET /api/v1/estimates
func (h *EstimateHandler) GetEstimates(c *gin.Context) {
	userID := c.GetInt("user_id")
	companyIDStr := c.Query("company_id")
	clientIDStr := c.Query("client_id")

	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	if limit <= 0 {
		limit = 50
	}
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	if offset < 0 {
		offset = 0
	}

	query := `
		SELECT
			e.id, e.company_id, e.client_id, e.estimate_number,
			e.estimate_date, e.expiry_date, e.subtotal, e.tax, e.discount, e.total,
			e.status, e.converted_invoice_id, e.created_at,
			COALESCE(c.name, '')
		FROM estimates e
		JOIN clients c ON c.id = e.client_id
		WHERE e.user_id = $1
	`
	args := []interface{}{userID}
	argPos := 2

	if companyIDStr != "" {
		if companyID, err := strconv.Atoi(companyIDStr); err == nil {
			query += ` AND e.company_id = $` + strconv.Itoa(argPos)
			args = append(args, companyID)
			argPos++
		}
	}
	if clientIDStr != "" {
		if clientID, err := strconv.Atoi(clientIDStr); err == nil {
			query += ` AND e.client_id = $` + strconv.Itoa(argPos)
			args = append(args, clientID)
			argPos++
		}
	}
	query += ` ORDER BY e.estimate_date DESC LIMIT $` + strconv.Itoa(argPos) + ` OFFSET $` + strconv.Itoa(argPos+1)
	args = append(args, limit, offset)

	rows, err := h.db.DB.Query(query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch estimates"})
		return
	}
	defer rows.Close()

	estimates := []gin.H{}
	for rows.Next() {
		var (
			id, companyID, clientID          int
			estimateNumber, status           string
			estimateDate                     time.Time
			expiryDate                       sql.NullTime
			subtotal, tax, discount, total   float64
			convertedInvoiceID               sql.NullInt64
			createdAt                        time.Time
			clientName                       string
		)
		if err := rows.Scan(
			&id, &companyID, &clientID, &estimateNumber,
			&estimateDate, &expiryDate, &subtotal, &tax, &discount, &total,
			&status, &convertedInvoiceID, &createdAt, &clientName,
		); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to scan estimate"})
			return
		}

		row := gin.H{
			"id":               id,
			"company_id":       companyID,
			"client_id":        clientID,
			"client_name":      clientName,
			"estimate_number":  estimateNumber,
			"estimate_date":    estimateDate.Format("2006-01-02"),
			"subtotal":         subtotal,
			"tax":              tax,
			"discount":         discount,
			"total":            total,
			"status":           status,
			"created_at":       createdAt,
		}
		if expiryDate.Valid {
			row["expiry_date"] = expiryDate.Time.Format("2006-01-02")
		} else {
			row["expiry_date"] = nil
		}
		if convertedInvoiceID.Valid {
			row["converted_invoice_id"] = convertedInvoiceID.Int64
		} else {
			row["converted_invoice_id"] = nil
		}
		estimates = append(estimates, row)
	}

	c.JSON(http.StatusOK, gin.H{"data": estimates})
}

// GET /api/v1/estimates/:id
func (h *EstimateHandler) GetEstimateByID(c *gin.Context) {
	userID := c.GetInt("user_id")
	estimateID := c.Param("id")

	var (
		id, clientID                    int
		estimateNumber, status          string
		estimateDate                    time.Time
		expiryDate                      sql.NullTime
		subtotal, tax, discount, total  float64
		convertedInvoiceID              sql.NullInt64
		createdAt                       time.Time
		clientName                      string
	)

	err := h.db.DB.QueryRow(`
		SELECT
			e.id, e.client_id, e.estimate_number, e.estimate_date, e.expiry_date,
			e.subtotal, e.tax, e.discount, e.total, e.status, e.converted_invoice_id,
			e.created_at, c.name
		FROM estimates e
		JOIN clients c ON c.id = e.client_id
		WHERE e.id = $1 AND e.user_id = $2
	`, estimateID, userID).Scan(
		&id, &clientID, &estimateNumber, &estimateDate, &expiryDate,
		&subtotal, &tax, &discount, &total, &status, &convertedInvoiceID,
		&createdAt, &clientName,
	)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Estimate not found"})
		return
	}

	rows, err := h.db.DB.Query(`
		SELECT id, item_id, qty, rate, discount, tax_rate, total
		FROM estimate_items
		WHERE estimate_id = $1
		ORDER BY id
	`, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch estimate items"})
		return
	}
	defer rows.Close()

	items := []gin.H{}
	for rows.Next() {
		var (
			itemRowID, itemID, qty             int
			rate, itemDiscount, taxRate, total float64
		)
		if err := rows.Scan(&itemRowID, &itemID, &qty, &rate, &itemDiscount, &taxRate, &total); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to scan estimate item"})
			return
		}
		items = append(items, gin.H{
			"id": itemRowID, "item_id": itemID, "qty": qty,
			"rate": rate, "discount": itemDiscount, "tax_rate": taxRate, "total": total,
		})
	}

	resp := gin.H{
		"id":              id,
		"estimate_number": estimateNumber,
		"estimate_date":   estimateDate.Format("2006-01-02"),
		"subtotal":        subtotal,
		"tax":             tax,
		"discount":        discount,
		"total":           total,
		"status":          status,
		"created_at":      createdAt,
		"client":          gin.H{"id": clientID, "name": clientName},
		"items":           items,
	}
	if expiryDate.Valid {
		resp["expiry_date"] = expiryDate.Time.Format("2006-01-02")
	} else {
		resp["expiry_date"] = nil
	}
	if convertedInvoiceID.Valid {
		resp["converted_invoice_id"] = convertedInvoiceID.Int64
	} else {
		resp["converted_invoice_id"] = nil
	}

	c.JSON(http.StatusOK, resp)
}

// PUT /api/v1/estimates/:id/update
func (h *EstimateHandler) UpdateEstimate(c *gin.Context) {
	estimateID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid estimate id"})
		return
	}
	userID := c.GetInt("user_id")

	var req models.UpdateEstimateRequestDTO
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid input", "detail": err.Error()})
		return
	}
	if len(req.Items) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Estimate must contain at least one item"})
		return
	}

	var (
		companyID int
		status    string
	)
	err = h.db.DB.QueryRow(`SELECT company_id, status FROM estimates WHERE id = $1 AND user_id = $2`, estimateID, userID).Scan(&companyID, &status)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Estimate not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch estimate"})
		return
	}
	if status != "draft" {
		c.JSON(http.StatusConflict, gin.H{"error": "Only draft estimates can be edited"})
		return
	}

	var clientExists bool
	err = h.db.DB.QueryRow(`
		SELECT EXISTS (SELECT 1 FROM clients WHERE id = $1 AND user_id = $2 AND company_id = $3)
	`, req.ClientID, userID, companyID).Scan(&clientExists)
	if err != nil || !clientExists {
		c.JSON(http.StatusForbidden, gin.H{"error": "Invalid or unauthorized client"})
		return
	}

	var subtotal, taxTotal float64
	for _, item := range req.Items {
		lineAfterDiscount := (item.Rate * float64(item.Qty)) - item.Discount
		subtotal += lineAfterDiscount
		taxTotal += lineAfterDiscount * (item.TaxRate / 100)
	}
	preDiscountTotal := subtotal + taxTotal
	discount := req.Discount
	if discount < 0 {
		discount = 0
	}
	if discount > preDiscountTotal {
		discount = preDiscountTotal
	}
	total := preDiscountTotal - discount

	estimateDate, err := time.Parse("2006-01-02", req.EstimateDate)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid estimate_date"})
		return
	}
	var expiryDate *time.Time
	if req.ExpiryDate != nil && *req.ExpiryDate != "" {
		parsed, err := time.Parse("2006-01-02", *req.ExpiryDate)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid expiry_date"})
			return
		}
		expiryDate = &parsed
	}

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

	_, err = tx.Exec(`
		UPDATE estimates
		SET client_id = $1, estimate_date = $2, expiry_date = $3,
		    subtotal = $4, tax = $5, discount = $6, total = $7, updated_at = NOW()
		WHERE id = $8
	`, req.ClientID, estimateDate, expiryDate, subtotal, taxTotal, discount, total, estimateID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update estimate"})
		return
	}

	_, err = tx.Exec(`DELETE FROM estimate_items WHERE estimate_id = $1`, estimateID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to clear estimate items"})
		return
	}

	for _, item := range req.Items {
		lineTotal := (item.Rate * float64(item.Qty)) - item.Discount
		lineTotal += lineTotal * (item.TaxRate / 100)
		_, err = tx.Exec(`
			INSERT INTO estimate_items (estimate_id, item_id, qty, rate, discount, tax_rate, total)
			VALUES ($1,$2,$3,$4,$5,$6,$7)
		`, estimateID, item.ItemID, item.Qty, item.Rate, item.Discount, item.TaxRate, lineTotal)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to add estimate items"})
			return
		}
	}

	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to commit transaction"})
		return
	}
	committed = true

	c.JSON(http.StatusOK, gin.H{"message": "Estimate updated"})
}

// POST /api/v1/estimates/:id/status
func (h *EstimateHandler) UpdateEstimateStatus(c *gin.Context) {
	estimateID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid estimate id"})
		return
	}
	userID := c.GetInt("user_id")

	var req models.EstimateStatusUpdateDTO
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid input"})
		return
	}

	allowed := map[string]bool{"sent": true, "accepted": true, "rejected": true}
	if !allowed[req.Status] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Status must be one of: sent, accepted, rejected"})
		return
	}

	result, err := h.db.DB.Exec(`
		UPDATE estimates SET status = $1, updated_at = NOW()
		WHERE id = $2 AND user_id = $3 AND status != 'converted'
	`, req.Status, estimateID, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update status"})
		return
	}
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Estimate not found or already converted"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Status updated"})
}

// POST /api/v1/estimates/:id/convert
func (h *EstimateHandler) ConvertToInvoice(c *gin.Context) {
	estimateID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid estimate id"})
		return
	}
	userID := c.GetInt("user_id")

	var (
		companyID, clientID int
		status               string
		discount             float64
	)
	err = h.db.DB.QueryRow(`
		SELECT company_id, client_id, status, discount FROM estimates WHERE id = $1 AND user_id = $2
	`, estimateID, userID).Scan(&companyID, &clientID, &status, &discount)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Estimate not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch estimate"})
		return
	}
	if status == "converted" {
		c.JSON(http.StatusConflict, gin.H{"error": "Estimate already converted"})
		return
	}

	itemRows, err := h.db.DB.Query(`
		SELECT item_id, qty, rate, discount, tax_rate FROM estimate_items WHERE estimate_id = $1
	`, estimateID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch estimate items"})
		return
	}
	defer itemRows.Close()

	var subtotal, taxTotal float64
	type lineItem struct {
		itemID              int
		qty                 int
		rate, discount, tax float64
	}
	var lines []lineItem
	for itemRows.Next() {
		var li lineItem
		if err := itemRows.Scan(&li.itemID, &li.qty, &li.rate, &li.discount, &li.tax); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to scan estimate item"})
			return
		}
		lineAfterDiscount := (li.rate * float64(li.qty)) - li.discount
		subtotal += lineAfterDiscount
		taxTotal += lineAfterDiscount * (li.tax / 100)
		lines = append(lines, li)
	}
	if len(lines) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Estimate has no items"})
		return
	}

	preDiscountTotal := subtotal + taxTotal
	if discount > preDiscountTotal {
		discount = preDiscountTotal
	}
	total := preDiscountTotal - discount

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

	invDate := time.Now()
	fy := utils.FinancialYear(invDate)
	var nextNumber int
	err = tx.QueryRow(`
		INSERT INTO invoice_counters (company_id, financial_year)
		VALUES ($1, $2)
		ON CONFLICT (company_id, financial_year)
		DO UPDATE SET next_number = invoice_counters.next_number + 1
		RETURNING next_number
	`, companyID, fy).Scan(&nextNumber)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate invoice number"})
		return
	}
	invoiceNumber := fmt.Sprintf("INV/%s/%04d", fy, nextNumber)
	dueDate := invDate.AddDate(0, 0, 7)

	var invoiceID int
	err = tx.QueryRow(`
		INSERT INTO invoices (
			company_id, user_id, client_id, invoice_number, invoice_date, due_date,
			subtotal, tax, discount, total, status, paid_amount, remaining_amount
		)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,'draft',0,$10)
		RETURNING id
	`, companyID, userID, clientID, invoiceNumber, invDate, dueDate, subtotal, taxTotal, discount, total).Scan(&invoiceID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create invoice", "detail": err.Error()})
		return
	}

	for _, li := range lines {
		lineTotal := (li.rate * float64(li.qty)) - li.discount
		lineTotal += lineTotal * (li.tax / 100)
		_, err = tx.Exec(`
			INSERT INTO invoice_items (invoice_id, item_id, qty, rate, discount, tax_rate, total)
			VALUES ($1,$2,$3,$4,$5,$6,$7)
		`, invoiceID, li.itemID, li.qty, li.rate, li.discount, li.tax, lineTotal)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to copy items to invoice"})
			return
		}
	}

	_, err = tx.Exec(`
		UPDATE estimates SET status = 'converted', converted_invoice_id = $1, updated_at = NOW()
		WHERE id = $2
	`, invoiceID, estimateID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to mark estimate converted"})
		return
	}

	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to commit transaction"})
		return
	}
	committed = true

	c.JSON(http.StatusOK, gin.H{
		"invoice_id":     invoiceID,
		"invoice_number": invoiceNumber,
	})
}

// GET /api/v1/estimates/number-preview
func (h *EstimateHandler) GetEstimateNumberPreview(c *gin.Context) {
	userID := c.GetInt("user_id")
	companyIDStr := c.Query("company_id")
	if companyIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "company_id is required"})
		return
	}
	companyID, err := strconv.Atoi(companyIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid company_id"})
		return
	}

	owned, err := companyBelongsToUser(h.db.DB, int64(companyID), userID)
	if err != nil || !owned {
		c.JSON(http.StatusForbidden, gin.H{"error": "Unauthorized"})
		return
	}

	fy := utils.FinancialYear(time.Now())
	var nextNumber int
	err = h.db.DB.QueryRow(`
		SELECT COALESCE(next_number, 0) + 1 FROM estimate_counters WHERE company_id = $1 AND financial_year = $2
	`, companyID, fy).Scan(&nextNumber)
	if err != nil {
		nextNumber = 1
	}

	c.JSON(http.StatusOK, gin.H{
		"preview": fmt.Sprintf("EST/%s/%04d", fy, nextNumber),
	})
}

// GET /api/v1/estimates/:id/pdf
func (h *EstimateHandler) GetEstimatePDF(c *gin.Context) {
	estimateID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid estimate id"})
		return
	}
	userID := c.GetInt("user_id")

	var owned bool
	err = h.db.DB.QueryRow(`
		SELECT EXISTS (SELECT 1 FROM estimates WHERE id = $1 AND user_id = $2)
	`, estimateID, userID).Scan(&owned)
	if err != nil || !owned {
		c.JSON(http.StatusForbidden, gin.H{"error": "Unauthorized"})
		return
	}

	template := c.DefaultQuery("template", pdf.TemplateClassic)

	data, err := services.FetchEstimatePDFData(h.db.DB, estimateID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch estimate data"})
		return
	}

	pdfBytes, err := pdf.GenerateInvoicePDF(data, "original", template)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate PDF"})
		return
	}

	fileName := fmt.Sprintf("Estimate_%s.pdf", data.Invoice.InvoiceNumber)
	c.Header("Content-Type", "application/pdf")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", fileName))
	c.Data(http.StatusOK, "application/pdf", pdfBytes)
}
