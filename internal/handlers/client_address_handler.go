package handlers

import (
	"fmt"
	database "invo-server/internal/db"
	"invo-server/internal/models"
	"log"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

type ClientAddressHandler struct {
	db *database.Database
}

func NewClientAddressHandler(db *database.Database) *ClientAddressHandler {
	return &ClientAddressHandler{db: db}
}

func (h *ClientAddressHandler) SaveClientAddress(c *gin.Context) {
	clientID, _ := strconv.Atoi(c.Param("clientId"))
	userID := c.GetInt("user_id")

	var req models.Address
	if err := c.ShouldBindJSON(&req); err != nil {
		fmt.Println(err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid input"})
		return
	}

	if req.AddressType == "billing" && req.Line1 == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Billing address line1 is required"})
		return
	}

	_, err := h.db.DB.Exec(`
		INSERT INTO client_addresses (
			client_id, type,
			name, line1, line2, city, state,
			postal_code, country, phone, email, gst_number
		)
		SELECT
			$1, $2,
			$3,$4,$5,$6,$7,$8,$9,$10,$11,$12
		WHERE EXISTS (
			SELECT 1 FROM clients
			WHERE id = $1 AND user_id = $13
		)
		ON CONFLICT (client_id, type)
		DO UPDATE SET
			name = EXCLUDED.name,
			line1 = EXCLUDED.line1,
			line2 = EXCLUDED.line2,
			city = EXCLUDED.city,
			state = EXCLUDED.state,
			postal_code = EXCLUDED.postal_code,
			country = EXCLUDED.country,
			phone = EXCLUDED.phone,
			email = EXCLUDED.email,
			gst_number = EXCLUDED.gst_number,
			updated_at = NOW()
	`,
		clientID,
		req.AddressType, // "billing" | "shipping"
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
		log.Println("DB ERROR:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Client address saved"})
}

func (h *ClientAddressHandler) GetClientAddress(c *gin.Context) {
	clientID, _ := strconv.Atoi(c.Param("clientId"))
	userID := c.GetInt("user_id")
	addrType := c.Query("type") // billing | shipping

	if addrType != "billing" && addrType != "shipping" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "type must be billing or shipping",
		})
		return
	}

	var address models.Address

	err := h.db.DB.QueryRow(`
		SELECT type, name, line1, line2, city, state,
		       postal_code, country, phone, email, gst_number
		FROM client_addresses
		WHERE client_id = $1 AND type = $2
		  AND EXISTS (
			  SELECT 1 FROM clients
			  WHERE id = $1 AND user_id = $3
		  )
	`, clientID, addrType, userID).Scan(
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

	log.Println("Fetched client address:", address)

	c.JSON(http.StatusOK, gin.H{"data": address})
}
