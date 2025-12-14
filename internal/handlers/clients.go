package handlers

import (
	"fmt"
	database "invo-server/internal/db"
	"invo-server/internal/models"
	"net/http"

	"github.com/gin-gonic/gin"
)

type clientHandler struct {
	db *database.Database
}

func NewClientHandler(db *database.Database) *clientHandler {
	return &clientHandler{db: db}
}

// CreateClient handles creating a new client
func (h *clientHandler) CreateClient(c *gin.Context) {

	var request = models.Client{}

	userID := c.GetInt("user_id")

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid input"})
		return
	}

	// Ensure company belongs to this user
	var exists bool
	h.db.DB.QueryRow(`
        SELECT EXISTS(
            SELECT 1 FROM companies 
            WHERE id=$1 AND user_id=$2
        )`, request.CompanyID, userID).Scan(&exists)

	if !exists {
		c.JSON(403, gin.H{"error": "Unauthorized company access"})
		return
	}

	// Now insert the client
	_, err := h.db.DB.Exec(`
        INSERT INTO clients (name, email, phone, address, city, state, pincode, company_id, user_id) 
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
    `, request.Name, request.Email, request.Phone, request.Address, request.City, request.State, request.Pincode, request.CompanyID, userID)

	if err != nil {
		fmt.Println("SQL ERROR:", err)
		c.JSON(500, gin.H{"error": "Failed to create client", "detail": err.Error()})
		return
	}

	fmt.Println(err)

	fmt.Println("Client created for user:", err)
	c.JSON(201, gin.H{"message": "Client created"})
}

// GET /api/v1/companies/:id/clients
func (h *clientHandler) GetClients(c *gin.Context) {
	userID := c.GetInt("user_id")
	companyID := c.Param("id")

	// 1️⃣ Check if this company belongs to this user
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

	// 2️⃣ Fetch clients
	rows, err := h.db.DB.Query(`
        SELECT id, name, email, phone, address, city, state, pincode 
        FROM clients
        WHERE company_id=$1
    `, companyID)

	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to fetch clients"})
		return
	}

	defer rows.Close()

	clients := []models.Client{}
	for rows.Next() {
		var cl models.Client
		rows.Scan(&cl.ID, &cl.Name, &cl.Email, &cl.Phone, &cl.Address, &cl.City, &cl.State, &cl.Pincode)
		clients = append(clients, cl)
	}

	c.JSON(200, gin.H{"clients": clients})
}
