package handlers

import (
	database "invo-server/internal/db"
	"invo-server/internal/models"
	"log"
	"net/http"
	"strconv"
	"strings"

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
            SELECT 1 FROM companies WHERE id = $1 AND user_id = $2
            UNION ALL
            SELECT 1 FROM company_members WHERE company_id = $1 AND user_id = $2
        )`, request.CompanyID, userID).Scan(&exists)

	if !exists {
		c.JSON(403, gin.H{"error": "Unauthorized company access"})
		return
	}

	// The new id is returned because creating a client is rarely the whole job: an
	// invoice needs the client's billing address, which is stored separately, and
	// without the id the caller had no way to attach one to the client it just made.
	var clientID int
	err := h.db.DB.QueryRow(`
        INSERT INTO clients (name, email, phone, address, city, state, pincode, company_id, user_id)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
        RETURNING id
    `, request.Name, request.Email, request.Phone, request.Address, request.City, request.State, request.Pincode, request.CompanyID, userID).Scan(&clientID)

	if err != nil {
		log.Println("failed to create client:", err)
		c.JSON(500, gin.H{"error": "Failed to create client"})
		return
	}

	c.JSON(201, gin.H{"message": "Client created", "client_id": clientID})
}

// GET /api/v1/companies/:id/clients
func (h *clientHandler) GetClients(c *gin.Context) {
	userID := c.GetInt("user_id")
	companyID := c.Param("companyId")

	// 1️⃣ Check if this company belongs to this user
	var exists bool
	h.db.DB.QueryRow(`
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

	// 2️⃣ Fetch clients.
	//
	// Every optional column is coalesced. These are all nullable, a quick-sale Cash or
	// UPI client is created with most of them blank, and the client app declares them
	// as non-optional strings — so a single null would fail decoding for the whole
	// list, not just the row it came from.
	search := strings.TrimSpace(c.Query("search"))

	query := `
        SELECT id, name,
               COALESCE(email, ''), COALESCE(phone, ''), COALESCE(address, ''),
               COALESCE(city, ''), COALESCE(state, ''), COALESCE(pincode, '')
        FROM clients
        WHERE company_id=$1
    `
	args := []interface{}{companyID}

	if search != "" {
		query += " AND (name ILIKE $2 OR COALESCE(phone, '') ILIKE $2 OR COALESCE(email, '') ILIKE $2)"
		args = append(args, "%"+search+"%")
	}

	// Paged, like invoices and items.
	//
	// This returned every client a company had, in one reply, every time a screen
	// opened — fine at thirty, a large download and a long render at three thousand,
	// and the database did that work whether or not anybody scrolled. A caller that
	// sends no limit still gets the whole list, so the apps in the store keep working;
	// the name and id ordering makes paging stable for one that does.
	//
	// id as a tiebreaker: two clients can share a name, and without it one of them
	// could appear on two pages or on none.
	query += " ORDER BY name, id"
	if limit := clampPageSize(mustAtoi(c.Query("limit")), 0); limit > 0 {
		query += " LIMIT $" + strconv.Itoa(len(args)+1) + " OFFSET $" + strconv.Itoa(len(args)+2)
		args = append(args, limit, mustAtoi(c.Query("offset")))
	}

	rows, err := h.db.DB.Query(query, args...)
	if err != nil {
		log.Println("failed to fetch clients:", err)
		c.JSON(500, gin.H{"error": "Failed to fetch clients"})
		return
	}

	defer rows.Close()

	clients := []models.Client{}
	for rows.Next() {
		var cl models.Client
		// The scan error was previously discarded entirely, so a failure appended a
		// half-populated client rather than reporting anything.
		if err := rows.Scan(
			&cl.ID, &cl.Name, &cl.Email, &cl.Phone,
			&cl.Address, &cl.City, &cl.State, &cl.Pincode,
		); err != nil {
			log.Println("failed to scan client:", err)
			c.JSON(500, gin.H{"error": "Failed to fetch clients"})
			return
		}
		clients = append(clients, cl)
	}
	if err := rows.Err(); err != nil {
		log.Println("failed to read clients:", err)
		c.JSON(500, gin.H{"error": "Failed to fetch clients"})
		return
	}

	c.JSON(200, gin.H{"clients": clients})
}

// UpdateClient edits a client. PUT /api/v1/clients/:clientId
//
// Without this a typo in a customer's name, GSTIN or address was permanent and
// repeated on every future invoice, since invoices snapshot the client's details at
// the time they are raised.
func (h *clientHandler) UpdateClient(c *gin.Context) {
	clientID, err := strconv.Atoi(c.Param("clientId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid client id"})
		return
	}
	userID := c.GetInt("user_id")

	var request models.Client
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid input"})
		return
	}
	if strings.TrimSpace(request.Name) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Client name is required"})
		return
	}

	// Ownership is checked against the stored row, never a company id from the body.
	var owned bool
	if err := h.db.DB.QueryRow(`
		SELECT EXISTS(
			SELECT 1 FROM clients cl
			JOIN companies co ON co.id = cl.company_id
			WHERE cl.id = $1 AND co.id IN (SELECT company_id FROM companies_for_user($2))
		)
	`, clientID, userID).Scan(&owned); err != nil || !owned {
		c.JSON(http.StatusForbidden, gin.H{"error": "Unauthorized"})
		return
	}

	result, err := h.db.DB.Exec(`
		UPDATE clients
		SET name = $1, email = $2, phone = $3, address = $4,
		    city = $5, state = $6, pincode = $7, updated_at = NOW()
		WHERE id = $8
	`, request.Name, request.Email, request.Phone, request.Address,
		request.City, request.State, request.Pincode, clientID)
	if err != nil {
		log.Println("failed to update client:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update client"})
		return
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Client not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Client updated"})
}

// DeleteClient removes a client. DELETE /api/v1/clients/:clientId
//
// Refused once anything references them. Invoices snapshot the client's address but
// still carry client_id, and a ledger exists per client — deleting the row underneath
// would orphan financial history that has to remain auditable.
func (h *clientHandler) DeleteClient(c *gin.Context) {
	clientID, err := strconv.Atoi(c.Param("clientId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid client id"})
		return
	}
	userID := c.GetInt("user_id")

	var owned bool
	if err := h.db.DB.QueryRow(`
		SELECT EXISTS(
			SELECT 1 FROM clients cl
			JOIN companies co ON co.id = cl.company_id
			WHERE cl.id = $1 AND co.id IN (SELECT company_id FROM companies_for_user($2))
		)
	`, clientID, userID).Scan(&owned); err != nil || !owned {
		c.JSON(http.StatusForbidden, gin.H{"error": "Unauthorized"})
		return
	}

	var referenced bool
	if err := h.db.DB.QueryRow(`
		SELECT EXISTS(SELECT 1 FROM invoices WHERE client_id = $1)
		    OR EXISTS(SELECT 1 FROM estimates WHERE client_id = $1)
		    OR EXISTS(SELECT 1 FROM payments WHERE client_id = $1)
		    OR EXISTS(SELECT 1 FROM ledger_entries WHERE client_id = $1)
	`, clientID).Scan(&referenced); err != nil {
		log.Println("failed to check client references:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete client"})
		return
	}
	if referenced {
		c.JSON(http.StatusConflict, gin.H{
			"error": "This client has invoices or payments and cannot be deleted. Edit their details instead.",
		})
		return
	}

	if _, err := h.db.DB.Exec(`DELETE FROM clients WHERE id = $1`, clientID); err != nil {
		log.Println("failed to delete client:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete client"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Client deleted"})
}

// QuickSaleClient returns the company's Cash or UPI account, creating it if this is the
// first walk-in sale.
//
// POST /api/v1/companies/:companyId/quick-sale-client  {"account": "Cash"}
//
// The app used to do this by fetching every client and looking through the list, which
// is why the client list could never be paged: the account it needed might be on page
// four. Asking the server for it by name costs one indexed lookup and lets the list
// page like every other list.
//
// Find-or-create in one statement, so two tills asking at once end up with the same
// account rather than one each.
func (h *clientHandler) QuickSaleClient(c *gin.Context) {
	userID := c.GetInt("user_id")
	companyID := c.Param("companyId")

	var exists bool
	h.db.DB.QueryRowContext(c.Request.Context(), `
        SELECT EXISTS(
            SELECT 1 FROM companies WHERE id = $1 AND user_id = $2
            UNION ALL
            SELECT 1 FROM company_members WHERE company_id = $1 AND user_id = $2
        )
    `, companyID, userID).Scan(&exists)
	if !exists {
		c.JSON(http.StatusForbidden, gin.H{"error": "Unauthorized company access"})
		return
	}

	var req struct {
		Account string `json:"account"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid input"})
		return
	}

	// Only the two. Anything else would create an ordinary customer through a door
	// meant for walk-in accounts, and name it whatever the caller asked for.
	name := strings.TrimSpace(req.Account)
	switch strings.ToLower(name) {
	case "cash":
		name = "Cash"
	case "upi":
		name = "UPI"
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "Quick sale accounts are Cash and UPI."})
		return
	}

	// ON CONFLICT against the unique index, so a second caller gets the existing row
	// rather than an error or a duplicate. DO UPDATE rather than DO NOTHING because
	// DO NOTHING returns no row at all when the account is already there.
	var client models.Client
	err := h.db.DB.QueryRowContext(c.Request.Context(), `
        INSERT INTO clients (name, email, phone, address, city, state, pincode, company_id, user_id)
        VALUES ($1, '', '', '', '', '', '', $2, $3)
        ON CONFLICT (company_id, lower(name)) WHERE lower(name) IN ('cash', 'upi')
        DO UPDATE SET updated_at = NOW()
        RETURNING id, name, COALESCE(email, ''), COALESCE(phone, ''), COALESCE(address, ''),
                  COALESCE(city, ''), COALESCE(state, ''), COALESCE(pincode, '')
    `, name, companyID, userID).Scan(
		&client.ID, &client.Name, &client.Email, &client.Phone,
		&client.Address, &client.City, &client.State, &client.Pincode,
	)
	if err != nil {
		log.Println("failed to resolve quick sale client:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to set up that account"})
		return
	}

	c.JSON(http.StatusOK, client)
}
