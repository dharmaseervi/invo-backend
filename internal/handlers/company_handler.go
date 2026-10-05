package handlers

import (
	"log"
	"net/http"
	"strconv"
	"strings"

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

	// The company and its owner's membership are written together. That membership row
	// is what grants access from here on, so a company saved without one would be a
	// business its own owner could not open.
	tx, err := h.db.DB.Begin()
	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to create company"})
		return
	}
	defer tx.Rollback()

	err = tx.QueryRow(
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
		log.Println("failed to create company:", err)
		c.JSON(500, gin.H{"error": "Failed to create company"})
		return
	}

	if _, err := tx.Exec(`
		INSERT INTO company_members (company_id, user_id, role, name)
		VALUES ($1, $2, 'owner', '')
		ON CONFLICT (company_id, user_id) DO NOTHING
	`, newID, userID); err != nil {
		log.Println("failed to record company owner:", err)
		c.JSON(500, gin.H{"error": "Failed to create company"})
		return
	}

	if err := tx.Commit(); err != nil {
		log.Println("failed to commit company creation:", err)
		c.JSON(500, gin.H{"error": "Failed to create company"})
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

	// Every business this person works in, not only the ones they own — a counter boy
	// has no company of his own, and without this his app would open on an empty shelf.
	// An owner sees exactly what they saw before, now with the word for what they are.
	rows, err := h.db.DB.Query(`
        SELECT c.id, c.user_id, c.name, c.address, c.phone, c.gst, c.city, c.state, c.pincode,
               COALESCE(m.role, CASE WHEN c.user_id = $1 THEN 'owner' ELSE '' END)
        FROM companies c
        LEFT JOIN company_members m ON m.company_id = c.id AND m.user_id = $1
        WHERE c.user_id = $1 OR m.id IS NOT NULL
        ORDER BY c.id
    `, userID)

	if err != nil {
		c.JSON(500, gin.H{"error": "Database error"})
		return
	}
	defer rows.Close()

	companies := []models.Company{}

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
			&company.Role,
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

// UpdateCompany edits a company's own details. PUT /api/v1/companies/:companyId
//
// These fields are what every invoice prints as the seller — the PDF reads them from
// this row directly. Without an update, a company created with a typo in its name,
// GSTIN or state carried that typo onto every invoice forever, and the state is not
// cosmetic: it decides CGST+SGST against IGST on every supply.
func (h *CompanyHandler) UpdateCompany(c *gin.Context) {
	companyID, err := strconv.ParseInt(c.Param("companyId"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid company id"})
		return
	}
	userID := c.GetInt("user_id")

	var request models.Company
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid input"})
		return
	}
	if strings.TrimSpace(request.Name) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Company name is required"})
		return
	}

	// Ownership comes from the stored row, never from the body.
	result, err := h.db.DB.Exec(`
		UPDATE companies
		SET name = $1, address = $2, phone = $3, gst = $4,
		    city = $5, state = $6, pincode = $7
		WHERE id = $8 AND user_id = $9
	`,
		strings.TrimSpace(request.Name),
		request.Address,
		request.Phone,
		request.Gst,
		request.City,
		request.State,
		request.Pincode,
		companyID,
		userID,
	)
	if err != nil {
		log.Println("failed to update company:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update company"})
		return
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		c.JSON(http.StatusForbidden, gin.H{"error": "Unauthorized"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Company updated"})
}
