package handlers

import (
	database "invo-server/internal/db"
	"invo-server/internal/models"

	"github.com/gin-gonic/gin"
)

type CompanyHandler struct {
	db *database.Database
}

func NewCompanyHandler(db *database.Database) *CompanyHandler {
	return &CompanyHandler{db: db}
}

func (h *CompanyHandler) CreateCompany(c *gin.Context) {
	var request models.Company

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(400, gin.H{"error": "Invalid input"})
		return
	}

	// Get user from middleware
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(401, gin.H{"error": "Unauthorized"})
		return
	}

	// Insert company
	query := `
        INSERT INTO companies (user_id, name, address, phone, gst, city, state, pincode)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
        RETURNING id
    `

	var newID int

	err := h.db.DB.QueryRow(
		query,
		userID,
		request.Name,
		request.Address,
		request.Phone,
		request.Gst,
		request.City,
		request.State,
		request.Pincode,
	).Scan(&newID)

	if err != nil {
		c.JSON(500, gin.H{"error": "Database error", "detail": err.Error()})
		return
	}

	c.JSON(201, gin.H{
		"message":    "Company created successfully",
		"company_id": newID,
	})
}

func (h *CompanyHandler) GetMyCompanies(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(401, gin.H{"error": "Unauthorized"})
		return
	}

	rows, err := h.db.DB.Query(`
        SELECT id, user_id, name, address, phone, gst, city, state, pincode
        FROM companies
        WHERE user_id = $1
    `, userID)

	if err != nil {
		c.JSON(500, gin.H{"error": "Database error"})
		return
	}
	defer rows.Close()

	var companies []models.Company

	for rows.Next() {
		var company models.Company
		if err := rows.Scan(
			&company.ID,
			&company.UserID,
			&company.Name,
			&company.Address,
			&company.Phone,
			&company.Gst,
			&company.City,
			&company.State,
			&company.Pincode,
		); err != nil {
			c.JSON(500, gin.H{"error": "Scan error"})
			return
		}
		companies = append(companies, company)
	}

	c.JSON(200, gin.H{
		"companies": companies,
	})
}
