package handlers

import (
	database "invo-server/internal/db"
	"invo-server/internal/models"
	"invo-server/internal/services"
	"log"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/lib/pq"
)

type itemHandler struct {
	db *database.Database
}

func NewItemHandler(db *database.Database) *itemHandler {
	return &itemHandler{db: db}
}

// isDuplicateSKU reports whether err is a violation of the (company_id, sku)
// unique index — i.e. this SKU is already used by another item in the company.
func isDuplicateSKU(err error) bool {
	if pqErr, ok := err.(*pq.Error); ok {
		return pqErr.Code == "23505" && pqErr.Constraint == "items_company_id_sku_unique"
	}
	return false
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
	// ✅ New
	_, err = h.db.DB.Exec(`
    INSERT INTO items 
    (name, category_id, sku, unit, description, cost_price, price, quantity, low_stock_alert, tax_rate, hsn_code, company_id, user_id) 
    VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
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
		request.HSNCode,
		request.CompanyID,
		userID,
	)

	if err != nil {
		if isDuplicateSKU(err) {
			c.JSON(http.StatusConflict, gin.H{"error": "This SKU is already used by another item"})
			return
		}
		log.Println("failed to create item:", err)
		c.JSON(500, gin.H{"error": "Failed to create item"})
		return
	}

	c.JSON(201, gin.H{"message": "Item created successfully"})
}

func (h *itemHandler) GetItems(c *gin.Context) {

	companyID := c.Param("companyId")
	userID := c.GetInt("user_id")

	// Validate company ownership
	var exists bool
	h.db.DB.QueryRow(`
        SELECT EXISTS(
            SELECT 1 FROM companies
            WHERE id=$1 AND user_id=$2
        )
    `, companyID, userID).Scan(&exists)

	if !exists {
		c.JSON(403, gin.H{"error": "Unauthorized company access"})
		return
	}

	// Fetch items
	rows, err := h.db.DB.Query(`
        SELECT 
        id, name, category_id, sku, unit, description,
        cost_price, price, quantity, low_stock_alert, tax_rate,
        hsn_code, company_id, user_id, created_at, updated_at
FROM items
WHERE company_id = $1
    `, companyID)

	if err != nil {
		log.Println("failed to fetch items:", err)
		c.JSON(500, gin.H{"error": "Failed to fetch items"})
		return
	}
	defer rows.Close()

	items := []models.Item{}

	for rows.Next() {
		var item models.Item
		// ✅ New Scan
		if err := rows.Scan(
			&item.ID, &item.Name, &item.CategoryID,
			&item.SKU, &item.Unit, &item.Description,
			&item.CostPrice, &item.Price, &item.Quantity,
			&item.LowStockAlert, &item.TaxRate,
			&item.HSNCode,
			&item.CompanyID, &item.UserID,
			&item.CreatedAt, &item.UpdatedAt,
		); err == nil {
			items = append(items, item)
		}
	}

	c.JSON(200, gin.H{"items": items})
}

// UpdateItem handles editing an existing item (name, stock, pricing, etc.)
func (h *itemHandler) UpdateItem(c *gin.Context) {
	itemID := c.Param("itemId")
	userID := c.GetInt("user_id")

	var request models.Item
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid input"})
		return
	}

	// Read the current quantity/company first so a manual stock change can be
	// logged to the audit trail — silently overwriting quantity with no record
	// of why is exactly the gap this closes.
	var previousQuantity, companyID int
	_ = h.db.DB.QueryRow(`
		SELECT quantity, company_id FROM items WHERE id = $1 AND user_id = $2
	`, itemID, userID).Scan(&previousQuantity, &companyID)

	result, err := h.db.DB.Exec(`
		UPDATE items SET
			name = $1, category_id = $2, sku = $3, unit = $4,
			description = $5, cost_price = $6, price = $7, quantity = $8,
			low_stock_alert = $9, tax_rate = $10, hsn_code = $11, updated_at = NOW()
		WHERE id = $12 AND user_id = $13
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
		request.HSNCode,
		itemID,
		userID,
	)

	if err != nil {
		if isDuplicateSKU(err) {
			c.JSON(http.StatusConflict, gin.H{"error": "This SKU is already used by another item"})
			return
		}
		log.Println("failed to update item:", err)
		c.JSON(500, gin.H{"error": "Failed to update item"})
		return
	}

	rows, _ := result.RowsAffected()
	if rows > 0 && request.Quantity != previousQuantity {
		id, _ := strconv.Atoi(itemID)
		if err := services.LogStockMovement(
			h.db.DB, id, companyID, userID, "adjustment",
			request.Quantity-previousQuantity, previousQuantity, request.Quantity,
			nil, nil,
		); err != nil {
			log.Println("failed to log stock movement:", err)
		}
	}
	if rows == 0 {
		c.JSON(http.StatusForbidden, gin.H{"error": "Item not found or unauthorized"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Item updated successfully"})
}

func (h *itemHandler) GetItemByID(c *gin.Context) {
	itemID := c.Param("itemId")
	userID := c.GetInt("user_id")

	var item models.Item

	err := h.db.DB.QueryRow(`
		SELECT 
    id, name, category_id, sku, unit, description,
    cost_price, price, quantity, low_stock_alert, tax_rate,
    hsn_code, company_id, user_id, created_at, updated_at
FROM items
WHERE id = $1 AND user_id = $2
	`, itemID, userID).Scan(
		&item.ID, &item.Name, &item.CategoryID,
		&item.SKU, &item.Unit, &item.Description,
		&item.CostPrice, &item.Price, &item.Quantity,
		&item.LowStockAlert, &item.TaxRate,
		&item.HSNCode,
		&item.CompanyID, &item.UserID,
		&item.CreatedAt, &item.UpdatedAt,
	)

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Item not found",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"items": []models.Item{item},
	})

}

// RestockItem records stock received from a supplier — increments quantity and
// logs it to the audit trail, distinct from a manual quantity edit.
func (h *itemHandler) RestockItem(c *gin.Context) {
	itemID := c.Param("itemId")
	userID := c.GetInt("user_id")

	var request models.RestockRequestDTO
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid input"})
		return
	}

	var previousQuantity, companyID int
	err := h.db.DB.QueryRow(`
		SELECT quantity, company_id FROM items WHERE id = $1 AND user_id = $2
	`, itemID, userID).Scan(&previousQuantity, &companyID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Item not found"})
		return
	}

	newQuantity := previousQuantity + request.Quantity

	_, err = h.db.DB.Exec(`
		UPDATE items SET quantity = $1, updated_at = NOW() WHERE id = $2 AND user_id = $3
	`, newQuantity, itemID, userID)
	if err != nil {
		log.Println("failed to restock item:", err)
		c.JSON(500, gin.H{"error": "Failed to restock item"})
		return
	}

	id, _ := strconv.Atoi(itemID)
	if err := services.LogStockMovement(
		h.db.DB, id, companyID, userID, "restock",
		request.Quantity, previousQuantity, newQuantity,
		request.Reference, request.Note,
	); err != nil {
		log.Println("failed to log stock movement:", err)
	}

	c.JSON(http.StatusOK, gin.H{
		"message":  "Stock updated",
		"quantity": newQuantity,
	})
}

// GetItemMovements returns an item's stock audit trail, newest first.
func (h *itemHandler) GetItemMovements(c *gin.Context) {
	itemID := c.Param("itemId")
	userID := c.GetInt("user_id")

	var owned bool
	h.db.DB.QueryRow(`
		SELECT EXISTS(SELECT 1 FROM items WHERE id = $1 AND user_id = $2)
	`, itemID, userID).Scan(&owned)
	if !owned {
		c.JSON(http.StatusForbidden, gin.H{"error": "Unauthorized"})
		return
	}

	rows, err := h.db.DB.Query(`
		SELECT id, item_id, movement_type, quantity_change, previous_quantity,
		       new_quantity, reference, note, TO_CHAR(created_at, 'DD Mon YYYY, HH12:MI AM')
		FROM stock_movements
		WHERE item_id = $1
		ORDER BY created_at DESC
		LIMIT 50
	`, itemID)
	if err != nil {
		log.Println("failed to fetch stock movements:", err)
		c.JSON(500, gin.H{"error": "Failed to fetch stock movements"})
		return
	}
	defer rows.Close()

	movements := []models.StockMovement{}
	for rows.Next() {
		var m models.StockMovement
		if err := rows.Scan(
			&m.ID, &m.ItemID, &m.MovementType, &m.QuantityChange,
			&m.PreviousQuantity, &m.NewQuantity, &m.Reference, &m.Note, &m.CreatedAt,
		); err == nil {
			movements = append(movements, m)
		}
	}

	c.JSON(http.StatusOK, gin.H{"movements": movements})
}
