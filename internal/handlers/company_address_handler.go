package handlers

import (
	database "invo-server/internal/db"
	"invo-server/internal/models"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

type CompanyAddressHandler struct {
	db *database.Database
}

func NewCompanyAddressHandler(db *database.Database) *CompanyAddressHandler {
	return &CompanyAddressHandler{db: db}
}

// GET company address
func (h *CompanyAddressHandler) GetCompanyAddress(c *gin.Context) {
	companyID, _ := strconv.Atoi(c.Param("companyId"))
	userID := c.GetInt("user_id")

	var address models.Address

	err := h.db.DB.QueryRow(`
		SELECT address_type, name, line1, line2, city, state,
		       postal_code, country, phone, email, gst_number
		FROM company_addresses
		WHERE owner_type='company'
		  AND owner_id=$1
		  AND EXISTS (
			  SELECT 1 FROM companies
			  WHERE id=$1 AND user_id=$2
		  )
	`, companyID, userID).Scan(
		&address.AddressType,
		&address.Name,
		&address.Line1,
		&address.Line2,
		&address.City,
		&address.State,
		&address.PostalCode,
		&address.Country,
		&address.Phone,
		&address.Email,
		&address.GSTNumber,
	)

	if err != nil {
		c.JSON(http.StatusOK, gin.H{"data": nil})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": address})
}

// CREATE / UPDATE company address
func (h *CompanyAddressHandler) SaveCompanyAddress(c *gin.Context) {
	companyID, _ := strconv.Atoi(c.Param("companyId"))
	userID := c.GetInt("user_id")

	var req models.Address
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid input"})
		return
	}

	_, err := h.db.DB.Exec(`
		INSERT INTO company_addresses (
			owner_type, owner_id, address_type,
			name, line1, line2, city, state,
			postal_code, country, phone, email, gst_number
		)
		SELECT
			'company', $1, $2,
			$3,$4,$5,$6,$7,$8,$9,$10,$11,$12
		WHERE EXISTS (
			SELECT 1 FROM companies WHERE id=$1 AND user_id=$13
		)
		ON CONFLICT (owner_type, owner_id, address_type)
		DO UPDATE SET
			name=EXCLUDED.name,
			line1=EXCLUDED.line1,
			line2=EXCLUDED.line2,
			city=EXCLUDED.city,
			state=EXCLUDED.state,
			postal_code=EXCLUDED.postal_code,
			country=EXCLUDED.country,
			phone=EXCLUDED.phone,
			email=EXCLUDED.email,
			gst_number=EXCLUDED.gst_number,
			updated_at=NOW()
	`,
		companyID,
		req.AddressType,
		req.Name,
		req.Line1,
		req.Line2,
		req.City,
		req.State,
		req.PostalCode,
		req.Country,
		req.Phone,
		req.Email,
		req.GSTNumber,
		userID,
	)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save address"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Company address saved"})
}
