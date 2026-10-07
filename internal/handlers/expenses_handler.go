package handlers

import (
	database "invo-server/internal/db"
	"invo-server/internal/models"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
)

type expenseHandler struct {
	db *database.Database
}

func NewExpenseHandler(db *database.Database) *expenseHandler {
	return &expenseHandler{db: db}
}

// CreateExpense handles creating a new expense
// POST /api/v1/expenses
func (h *expenseHandler) CreateExpense(c *gin.Context) {
	var request models.Expensess

	userID := c.GetInt("user_id")

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid input"})
		return
	}

	// Validate required fields
	if request.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Name is required"})
		return
	}

	if request.Amount < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Amount must be greater than or equal to 0"})
		return
	}

	if request.Date == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Date is required"})
		return
	}

	// Ensure company belongs to this user
	var exists bool
	h.db.DB.QueryRowContext(c.Request.Context(),
		`
        SELECT EXISTS(
            SELECT 1 FROM companies WHERE id = $1 AND user_id = $2
            UNION ALL
            SELECT 1 FROM company_members WHERE company_id = $1 AND user_id = $2
        )`, request.CompanyID, userID).Scan(&exists)

	if !exists {
		c.JSON(403, gin.H{"error": "Unauthorized company access"})
		return
	}

	// Insert the expense
	var expenseID int
	err := h.db.DB.QueryRowContext(c.Request.Context(),
		`
        INSERT INTO expensess (name, amount, description, date, company_id, user_id, payment_method) 
        VALUES ($1, $2, $3, $4, $5, $6, NULLIF($7, ''))
        RETURNING id
    `, request.Name, request.Amount, request.Description, request.Date, request.CompanyID, userID,
		request.PaymentMethod).Scan(&expenseID)

	if err != nil {
		log.Println("failed to create expense:", err)
		c.JSON(500, gin.H{"error": "Failed to create expense"})
		return
	}

	c.JSON(201, gin.H{
		"message": "Expense created successfully",
		"id":      expenseID,
	})
}

// GetExpenses retrieves all expenses for a company
// GET /api/v1/companies/:id/expenses
func (h *expenseHandler) GetExpenses(c *gin.Context) {
	userID := c.GetInt("user_id")
	companyID := c.Param("companyId")

	// Check if this company belongs to this user
	var exists bool
	h.db.DB.QueryRowContext(c.Request.Context(),
		`
        SELECT EXISTS(
            SELECT 1 FROM companies WHERE id = $1 AND user_id = $2
            UNION ALL
            SELECT 1 FROM company_members WHERE company_id = $1 AND user_id = $2
        )
    `, companyID, userID).Scan(&exists)

	if !exists {
		c.JSON(403, gin.H{"error": "Unauthorized company access"})
		return
	}

	// Fetch expenses. Paged when the caller asks; the whole list otherwise, so the
	// apps already in the store keep working. id breaks ties between expenses recorded
	// on the same date, without which a row could land on two pages or on none.
	query := `
        SELECT id, name, amount, description, date, COALESCE(payment_method, ''), created_at, updated_at
        FROM expensess
        WHERE company_id=$1
        ORDER BY date DESC, id DESC
    `
	args := []interface{}{companyID}
	if limit := clampPageSize(mustAtoi(c.Query("limit")), 0); limit > 0 {
		query += " LIMIT $2 OFFSET $3"
		args = append(args, limit, mustAtoi(c.Query("offset")))
	}

	rows, err := h.db.DB.QueryContext(c.Request.Context(),
		query, args...)

	if err != nil {
		log.Println("Query ERROR:", err)
		c.JSON(500, gin.H{"error": "Failed to fetch expenses"})
		return
	}

	defer rows.Close()

	expenses := []models.Expensess{}
	for rows.Next() {
		var exp models.Expensess
		err := rows.Scan(
			&exp.ID,
			&exp.Name,
			&exp.Amount,
			&exp.Description,
			&exp.Date,
			&exp.PaymentMethod,
			&exp.CreatedAt,
			&exp.UpdatedAt,
		)
		if err != nil {
			log.Println("Scan ERROR:", err)
			continue
		}
		expenses = append(expenses, exp)
	}

	if expenses == nil {
		expenses = []models.Expensess{}
	}

	c.JSON(200, gin.H{"expenses": expenses})
}

// GetExpenseByID retrieves a single expense by ID
// GET /api/v1/expenses/:id
func (h *expenseHandler) GetExpenseByID(c *gin.Context) {
	userID := c.GetInt("user_id")
	expenseID := c.Param("id")

	var exp models.Expensess
	var companyID int

	// Fetch expense and verify ownership
	err := h.db.DB.QueryRowContext(c.Request.Context(),
		`
        SELECT id, name, amount, description, date, company_id, COALESCE(payment_method, ''), created_at, updated_at
        FROM expensess
        WHERE id=$1
    `, expenseID).Scan(
		&exp.ID,
		&exp.Name,
		&exp.Amount,
		&exp.Description,
		&exp.Date,
		&companyID,
		&exp.PaymentMethod,
		&exp.CreatedAt,
		&exp.UpdatedAt,
	)

	if err != nil {
		c.JSON(404, gin.H{"error": "Expense not found"})
		return
	}

	// Verify this company belongs to the user
	var exists bool
	h.db.DB.QueryRowContext(c.Request.Context(),
		`
        SELECT EXISTS(
            SELECT 1 FROM companies WHERE id = $1 AND user_id = $2
            UNION ALL
            SELECT 1 FROM company_members WHERE company_id = $1 AND user_id = $2
        )
    `, companyID, userID).Scan(&exists)

	if !exists {
		c.JSON(403, gin.H{"error": "Unauthorized access"})
		return
	}

	exp.CompanyID = companyID
	c.JSON(200, gin.H{"expense": exp})
}

// UpdateExpense updates an existing expense
// PUT /api/v1/expenses/:id
func (h *expenseHandler) UpdateExpense(c *gin.Context) {
	userID := c.GetInt("user_id")
	expenseID := c.Param("id")

	var request struct {
		Name        string  `json:"name"`
		Amount      float64 `json:"amount"`
		Description string  `json:"description"`
		Date        string  `json:"date"`
		// Omitted keeps the existing method; an explicit empty string clears it.
		PaymentMethod *string `json:"payment_method"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid input"})
		return
	}

	// Get expense company
	var companyID int
	err := h.db.DB.QueryRowContext(c.Request.Context(),
		`
		SELECT company_id FROM expensess WHERE id=$1
	`, expenseID).Scan(&companyID)

	if err != nil {
		c.JSON(404, gin.H{"error": "Expense not found"})
		return
	}

	// Verify ownership
	var exists bool
	h.db.DB.QueryRowContext(c.Request.Context(),
		`
		SELECT EXISTS(
			SELECT 1 FROM companies WHERE id = $1 AND user_id = $2
            UNION ALL
            SELECT 1 FROM company_members WHERE company_id = $1 AND user_id = $2
		)
	`, companyID, userID).Scan(&exists)

	if !exists {
		c.JSON(403, gin.H{"error": "Unauthorized access"})
		return
	}

	// Validate amount
	if request.Amount < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Amount must be >= 0"})
		return
	}

	// Update safely
	_, err = h.db.DB.ExecContext(c.Request.Context(),
		`
		UPDATE expensess
		SET
			name = COALESCE($1, name),
			amount = COALESCE($2, amount),
			description = COALESCE($3, description),
			date = COALESCE($4, date),
			payment_method = CASE WHEN $5::text IS NULL THEN payment_method ELSE NULLIF($5, '') END,
			updated_at = NOW()
		WHERE id = $6
	`,
		request.Name,
		request.Amount,
		request.Description,
		request.Date,
		request.PaymentMethod,
		expenseID,
	)

	if err != nil {
		log.Println("SQL ERROR:", err)
		c.JSON(500, gin.H{"error": "Failed to update expense"})
		return
	}

	c.JSON(200, gin.H{"message": "Expense updated successfully"})
}

// DeleteExpense deletes an expense
// DELETE /api/v1/expenses/:id
func (h *expenseHandler) DeleteExpense(c *gin.Context) {
	userID := c.GetInt("user_id")
	expenseID := c.Param("id")

	// Get current expense and verify ownership
	var companyID int
	err := h.db.DB.QueryRowContext(c.Request.Context(),
		`
        SELECT company_id FROM expensess WHERE id=$1
    `, expenseID).Scan(&companyID)

	if err != nil {
		c.JSON(404, gin.H{"error": "Expense not found"})
		return
	}

	// Verify company belongs to user
	var exists bool
	h.db.DB.QueryRowContext(c.Request.Context(),
		`
        SELECT EXISTS(
            SELECT 1 FROM companies WHERE id = $1 AND user_id = $2
            UNION ALL
            SELECT 1 FROM company_members WHERE company_id = $1 AND user_id = $2
        )
    `, companyID, userID).Scan(&exists)

	if !exists {
		c.JSON(403, gin.H{"error": "Unauthorized access"})
		return
	}

	// Delete the expense
	_, err = h.db.DB.ExecContext(c.Request.Context(),
		`DELETE FROM expensess WHERE id=$1`, expenseID)

	if err != nil {
		log.Println("failed to delete expense:", err)
		c.JSON(500, gin.H{"error": "Failed to delete expense"})
		return
	}

	c.JSON(200, gin.H{"message": "Expense deleted successfully"})
}

// GetExpensesByDateRange retrieves expenses within a date range
// GET /api/v1/companies/:id/expenses/range?start_date=2024-01-01&end_date=2024-12-31
func (h *expenseHandler) GetExpensesByDateRange(c *gin.Context) {
	userID := c.GetInt("user_id")
	companyID := c.Param("id")
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")

	if startDate == "" || endDate == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "start_date and end_date query parameters are required"})
		return
	}

	// Check if this company belongs to this user
	var exists bool
	h.db.DB.QueryRowContext(c.Request.Context(),
		`
        SELECT EXISTS(
            SELECT 1 FROM companies WHERE id = $1 AND user_id = $2
            UNION ALL
            SELECT 1 FROM company_members WHERE company_id = $1 AND user_id = $2
        )
    `, companyID, userID).Scan(&exists)

	if !exists {
		c.JSON(403, gin.H{"error": "Unauthorized company access"})
		return
	}

	// Fetch expenses in date range
	rows, err := h.db.DB.QueryContext(c.Request.Context(),
		`
        SELECT id, name, amount, description, date, COALESCE(payment_method, ''), created_at, updated_at
        FROM expensess
        WHERE company_id=$1 AND date BETWEEN $2 AND $3
        ORDER BY date DESC
    `, companyID, startDate, endDate)

	if err != nil {
		log.Println("Query ERROR:", err)
		c.JSON(500, gin.H{"error": "Failed to fetch expenses"})
		return
	}

	defer rows.Close()

	expenses := []models.Expensess{}
	for rows.Next() {
		var exp models.Expensess
		err := rows.Scan(
			&exp.ID,
			&exp.Name,
			&exp.Amount,
			&exp.Description,
			&exp.Date,
			&exp.PaymentMethod,
			&exp.CreatedAt,
			&exp.UpdatedAt,
		)
		if err != nil {
			log.Println("Scan ERROR:", err)
			continue
		}
		expenses = append(expenses, exp)
	}

	if expenses == nil {
		expenses = []models.Expensess{}
	}

	c.JSON(200, gin.H{"expenses": expenses})
}

// GetExpenseStats retrieves expense statistics for a company
// GET /api/v1/companies/:id/expenses/stats
func (h *expenseHandler) GetExpenseStats(c *gin.Context) {
	userID := c.GetInt("user_id")
	companyID := c.Param("id")

	// Check if this company belongs to this user
	var exists bool
	h.db.DB.QueryRowContext(c.Request.Context(),
		`
        SELECT EXISTS(
            SELECT 1 FROM companies WHERE id = $1 AND user_id = $2
            UNION ALL
            SELECT 1 FROM company_members WHERE company_id = $1 AND user_id = $2
        )
    `, companyID, userID).Scan(&exists)

	if !exists {
		c.JSON(403, gin.H{"error": "Unauthorized company access"})
		return
	}

	var totalAmount float64
	var expenseCount int
	var avgAmount float64

	// Get stats
	err := h.db.DB.QueryRowContext(c.Request.Context(),
		`
        SELECT 
            COALESCE(SUM(amount), 0),
            COUNT(*),
            COALESCE(AVG(amount), 0)
        FROM expensess
        WHERE company_id=$1
    `, companyID).Scan(&totalAmount, &expenseCount, &avgAmount)

	if err != nil {
		log.Println("Query ERROR:", err)
		c.JSON(500, gin.H{"error": "Failed to fetch expense stats"})
		return
	}

	c.JSON(200, gin.H{
		"stats": gin.H{
			"total_amount":   totalAmount,
			"expense_count":  expenseCount,
			"average_amount": avgAmount,
		},
	})
}

// GetExpenseSummary returns the figures an expense screen shows above the rows: this
// month, last month, and the total.
//
// GET /api/v1/companies/:companyId/expenses/summary
//
// The apps summed the rows they had loaded. That was the whole list until expenses were
// paged, and a page afterwards — so a company that records more than a page of expenses
// in a month would have seen a month's total quietly under-report. The months are
// worked out in the database's own calendar, over every row.
func (h *expenseHandler) GetExpenseSummary(c *gin.Context) {
	userID := c.GetInt("user_id")
	companyID := c.Param("companyId")

	var exists bool
	h.db.DB.QueryRowContext(c.Request.Context(),
		`
        SELECT EXISTS(SELECT 1 FROM companies WHERE id = $1 AND user_id = $2
            UNION ALL
            SELECT 1 FROM company_members WHERE company_id = $1 AND user_id = $2)
    `, companyID, userID).Scan(&exists)

	if !exists {
		c.JSON(403, gin.H{"error": "Unauthorized company access"})
		return
	}

	var thisMonth, lastMonth, total float64
	var count int
	err := h.db.DB.QueryRowContext(c.Request.Context(),
		`
        SELECT
            COALESCE(SUM(amount) FILTER (
                WHERE date >= date_trunc('month', CURRENT_DATE)
                  AND date <  date_trunc('month', CURRENT_DATE) + INTERVAL '1 month'
            ), 0),
            COALESCE(SUM(amount) FILTER (
                WHERE date >= date_trunc('month', CURRENT_DATE) - INTERVAL '1 month'
                  AND date <  date_trunc('month', CURRENT_DATE)
            ), 0),
            COALESCE(SUM(amount), 0),
            COUNT(*)
        FROM expensess
        WHERE company_id=$1
    `, companyID).Scan(&thisMonth, &lastMonth, &total, &count)

	if err != nil {
		log.Println("Query ERROR:", err)
		c.JSON(500, gin.H{"error": "Failed to fetch expense summary"})
		return
	}

	c.JSON(200, gin.H{
		"this_month": thisMonth,
		"last_month": lastMonth,
		"total":      total,
		"count":      count,
	})
}
