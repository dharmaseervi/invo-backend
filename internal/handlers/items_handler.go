package handlers

import (
	"fmt"
	database "invo-server/internal/db"
	"invo-server/internal/models"
	"net/http"

	"github.com/gin-gonic/gin"
)

type itemHandler struct {
	db *database.Database
}

func NewItemHandler(db *database.Database) *itemHandler {
	return &itemHandler{db: db}
}

// CreateItem handles creating a new item
func (h *itemHandler) CreateItem(c *gin.Context) {

	var request models.Item

	userID := c.GetInt("user_id")

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid input"})
		return
	}

	// Ensure company belongs to this user
	var companyExists bool
	h.db.DB.QueryRow(`
		SELECT EXISTS(
			SELECT 1 FROM companies 
			WHERE id=$1 AND user_id=$2
		)
	`, request.CompanyID, userID).Scan(&companyExists)

	if !companyExists {
		c.JSON(403, gin.H{"error": "Unauthorized company access"})
		return
	}

	// Ensure category belongs to this user
	var categoryExists bool
	err := h.db.DB.QueryRow(`
		SELECT EXISTS(
			SELECT 1 FROM categories
			WHERE id=$1 AND user_id=$2
		)
	`, request.CategoryID, userID).Scan(&categoryExists)

	if err != nil || !categoryExists {
		c.JSON(403, gin.H{"error": "Invalid or unauthorized category"})
		return
	}

	// Insert the item
	_, err = h.db.DB.Exec(`
		INSERT INTO items 
		(name, category_id, sku, unit, description, cost_price, price, quantity, low_stock_alert, tax_rate, company_id, user_id) 
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
	`,
		request.Name,
		request.CategoryID,
		request.SKU,
		request.Unit,
		request.Description,
		request.CostPrice,
		request.Price,
		request.Quantity,
		request.LowStockAlert,
		request.TaxRate,
		request.CompanyID,
		userID,
	)

	if err != nil {
		fmt.Println("SQL ERROR:", err)
		c.JSON(500, gin.H{"error": "Failed to create item", "detail": err.Error()})
		return
	}

	c.JSON(201, gin.H{"message": "Item created successfully"})
}

func (h *itemHandler) GetItems(c *gin.Context) {

	companyID := c.Param("companyId")
	userID := c.GetInt("user_id")

	fmt.Println("Fetching items for company:", companyID, "by user:", userID)

	// Validate company ownership
	var exists bool
	h.db.DB.QueryRow(`
        SELECT EXISTS(
            SELECT 1 FROM companies
            WHERE id=$1 AND user_id=$2
        )
    `, companyID, userID).Scan(&exists)

	if !exists {
		fmt.Println("Unauthorized access attempt by user:", userID)
		c.JSON(403, gin.H{"error": "Unauthorized company access"})
		return
	}

	// Fetch items
	rows, err := h.db.DB.Query(`
        SELECT 
            id, name, category_id, sku, unit, description,
            cost_price, price, quantity, low_stock_alert, tax_rate,
            company_id, user_id, created_at, updated_at
        FROM items
        WHERE company_id = $1
        ORDER BY id DESC
    `, companyID)

	if err != nil {
		fmt.Println("SQL ERROR:", err)
		c.JSON(500, gin.H{"error": "Failed to fetch items"})
		return
	}
	defer rows.Close()

	var items []models.Item

	for rows.Next() {
		var item models.Item
		if err := rows.Scan(
			&item.ID,
			&item.Name,
			&item.CategoryID,
			&item.SKU,
			&item.Unit,
			&item.Description,
			&item.CostPrice,
			&item.Price,
			&item.Quantity,
			&item.LowStockAlert,
			&item.TaxRate,
			&item.CompanyID,
			&item.UserID,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err == nil {
			items = append(items, item)
		}
	}

	c.JSON(200, gin.H{
		"items": items,
	})
}
