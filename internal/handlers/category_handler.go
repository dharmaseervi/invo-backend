package handlers

import (
	"fmt"
	database "invo-server/internal/db"
	"invo-server/internal/models"
	"net/http"

	"github.com/gin-gonic/gin"
)

type CategoryHandler struct {
	db *database.Database
}

func NewCategoryHandler(db *database.Database) *CategoryHandler {
	return &CategoryHandler{db: db}
}

func (h *CategoryHandler) CreateCategory(c *gin.Context) {
	var request models.Category

	userID := c.GetInt("user_id")

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid input"})
		return
	}

	// Validate company belongs to user
	var exists bool
	h.db.DB.QueryRow(`
        SELECT EXISTS(
            SELECT 1 FROM companies 
            WHERE id=$1 AND user_id=$2
        )
    `, request.CompanyID, userID).Scan(&exists)

	if !exists {
		c.JSON(http.StatusForbidden, gin.H{"error": "Unauthorized company access"})
		return
	}

	// Insert category
	_, err := h.db.DB.Exec(`
        INSERT INTO categories (name, user_id, company_id)
        VALUES ($1, $2, $3)
    `, request.Name, userID, request.CompanyID)

	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to create category"})
		return
	}

	c.JSON(201, gin.H{"message": "Category created"})
}

func (h *CategoryHandler) GetCategories(c *gin.Context) {
	companyID := c.Param("companyId")
	userID := c.GetInt("user_id")

	// Verify ownership
	var exists bool
	h.db.DB.QueryRow(`
        SELECT EXISTS(
            SELECT 1 FROM companies
            WHERE id=$1 AND user_id=$2
        )
    `, companyID, userID).Scan(&exists)

	if !exists {
		fmt.Println("Unauthorized access attempt by user:", userID)
		c.JSON(http.StatusForbidden, gin.H{"error": "Unauthorized company access"})
		return
	}

	rows, err := h.db.DB.Query(`
        SELECT id, name, user_id, company_id
        FROM categories
        WHERE company_id = $1
        ORDER BY id DESC
    `, companyID)

	if err != nil {
		fmt.Println("SQL ERROR:", err)
		c.JSON(500, gin.H{"error": "Failed to fetch categories"})
		return
	}
	defer rows.Close()

	var categories []models.Category

	for rows.Next() {
		var cat models.Category
		if err := rows.Scan(
			&cat.ID, &cat.Name, &cat.UserID,
			&cat.CompanyID,
		); err == nil {
			categories = append(categories, cat)
		}
	}

	fmt.Println("Categories fetched for user:", categories)

	c.JSON(200, gin.H{
		"categories": categories,
	})
}
