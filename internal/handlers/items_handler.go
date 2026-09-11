package handlers

import (
	"encoding/base64"
	"fmt"
	database "invo-server/internal/db"
	"invo-server/internal/models"
	"invo-server/internal/services"
	"log"
	"net/http"
	"strconv"
	"strings"

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

	// limit is optional. Omitted means "everything", which keeps already-installed app
	// versions working; a client that sends it gets a page and a cursor for the next.
	rawLimit := mustAtoi(c.Query("limit"))
	paginated := rawLimit > 0
	limit := clampPageSize(rawLimit, maxPageSize)

	search := strings.TrimSpace(c.Query("search"))

	// Keyset, not OFFSET. Ordering is by name, and with OFFSET an item renamed or added
	// while the user scrolls shifts every later page, so rows get skipped or repeated.
	// The cursor carries the last row's (name, id), and (name, id) is unique and stable.
	cursorName, cursorID, cursorOK := decodeItemCursor(c.Query("cursor"))

	// COALESCE on every nullable column. Scanning a NULL into a plain string or int
	// fails, and because the old loop ignored scan errors, any item missing a unit, SKU,
	// HSN code or cost price was quietly dropped from the catalogue. category_id stays
	// nullable in the model, since "no category" is meaningful and 0 is not a category.
	query := `
        SELECT
        id, name, category_id,
        COALESCE(sku, ''), COALESCE(unit, ''), COALESCE(description, ''),
        COALESCE(cost_price, 0), price, COALESCE(quantity, 0),
        COALESCE(low_stock_alert, 0), COALESCE(tax_rate, 0),
        COALESCE(hsn_code, ''), company_id, user_id, created_at, updated_at
        FROM items
        WHERE company_id = $1
    `
	args := []interface{}{companyID}
	pos := 2

	if search != "" {
		// Matches name or SKU: at a thousand products a user searches for what is on
		// the label, which is as often the code as the name.
		query += fmt.Sprintf(" AND (name ILIKE $%d OR COALESCE(sku, '') ILIKE $%d)", pos, pos)
		args = append(args, "%"+search+"%")
		pos++
	}

	if cursorOK {
		query += fmt.Sprintf(" AND (name, id) > ($%d, $%d)", pos, pos+1)
		args = append(args, cursorName, cursorID)
		pos += 2
	}

	query += " ORDER BY name, id"

	if paginated {
		// One extra row reveals whether a further page exists without a second query.
		query += fmt.Sprintf(" LIMIT $%d", pos)
		args = append(args, limit+1)
	}

	rows, err := h.db.DB.Query(query, args...)
	if err != nil {
		log.Println("failed to fetch items:", err)
		c.JSON(500, gin.H{"error": "Failed to fetch items"})
		return
	}
	defer rows.Close()

	items := []models.Item{}

	for rows.Next() {
		var item models.Item
		if err := rows.Scan(
			&item.ID, &item.Name, &item.CategoryID,
			&item.SKU, &item.Unit, &item.Description,
			&item.CostPrice, &item.Price, &item.Quantity,
			&item.LowStockAlert, &item.TaxRate,
			&item.HSNCode,
			&item.CompanyID, &item.UserID,
			&item.CreatedAt, &item.UpdatedAt,
		); err != nil {
			// Previously a scan error skipped the row silently, so an item simply
			// disappeared from the list with no indication anything had gone wrong.
			log.Println("failed to scan item:", err)
			c.JSON(500, gin.H{"error": "Failed to fetch items"})
			return
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		log.Println("failed to read items:", err)
		c.JSON(500, gin.H{"error": "Failed to fetch items"})
		return
	}

	response := gin.H{"items": items}

	if paginated && len(items) > limit {
		items = items[:limit]
		last := items[len(items)-1]
		response["items"] = items
		response["next_cursor"] = encodeItemCursor(last.Name, last.ID)
	}

	c.JSON(200, response)
}

// Cursors are opaque to the client on purpose: it should hand back whatever it was
// given rather than construct one, so the ordering can change without breaking clients.
func encodeItemCursor(name string, id int) string {
	return base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf("%s\x1f%d", name, id)))
}

func decodeItemCursor(raw string) (string, int, bool) {
	if raw == "" {
		return "", 0, false
	}
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return "", 0, false
	}
	parts := strings.SplitN(string(decoded), "\x1f", 2)
	if len(parts) != 2 {
		return "", 0, false
	}
	id, err := strconv.Atoi(parts[1])
	if err != nil {
		return "", 0, false
	}
	return parts[0], id, true
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

	// CreateItem validates this; UpdateItem did not, so an item could be re-pointed at
	// another tenant's category and leak its name back through every item read.
	if request.CategoryID != nil && *request.CategoryID != 0 {
		var categoryOK bool
		if err := h.db.DB.QueryRow(`
			SELECT EXISTS(
				SELECT 1 FROM categories
				WHERE id = $1 AND user_id = $2 AND company_id = $3
			)
		`, *request.CategoryID, userID, companyID).Scan(&categoryOK); err != nil || !categoryOK {
			c.JSON(http.StatusForbidden, gin.H{"error": "Invalid or unauthorized category"})
			return
		}
	}

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
    id, name, category_id,
    COALESCE(sku, ''), COALESCE(unit, ''), COALESCE(description, ''),
    COALESCE(cost_price, 0), price, COALESCE(quantity, 0),
    COALESCE(low_stock_alert, 0), COALESCE(tax_rate, 0),
    COALESCE(hsn_code, ''), company_id, user_id, created_at, updated_at
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
