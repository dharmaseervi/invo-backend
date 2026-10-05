package handlers

import (
	"database/sql"
	"log"
	"net/http"
	"strconv"
	"strings"

	"invo-server/internal/middleware"
	utils "invo-server/internal/util"

	"github.com/gin-gonic/gin"
)

// Who works in a business, and what they are allowed to do.
//
// A shop that was one login is now a list of people. The owner makes an account for the
// counter boy, picks what he can reach, and takes it away again when he leaves — without
// anybody sharing a password, and with every invoice carrying the id of whoever wrote it.

type StaffHandler struct {
	db *sql.DB
}

func NewStaffHandler(db *sql.DB) *StaffHandler {
	return &StaffHandler{db: db}
}

// company reads company_id from the path. The permission middleware has already checked
// that the caller may manage staff here, so this only has to parse.
func (h *StaffHandler) company(c *gin.Context) (int64, bool) {
	companyID, err := strconv.ParseInt(c.Param("companyId"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "company_id is required"})
		return 0, false
	}
	return companyID, true
}

// GetStaff lists everybody who works in a business.
//
// GET /api/v1/companies/:companyId/staff
func (h *StaffHandler) GetStaff(c *gin.Context) {
	companyID, ok := h.company(c)
	if !ok {
		return
	}

	rows, err := h.db.Query(`
		SELECT m.id, m.user_id, u.email, m.name, m.role, TO_CHAR(m.created_at, 'YYYY-MM-DD')
		FROM company_members m
		JOIN users u ON u.id = m.user_id
		WHERE m.company_id = $1
		ORDER BY CASE m.role WHEN 'owner' THEN 0 WHEN 'manager' THEN 1 ELSE 2 END,
		         lower(m.name), m.id
	`, companyID)
	if err != nil {
		log.Println("failed to fetch staff:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch staff"})
		return
	}
	defer rows.Close()

	out := []gin.H{}
	for rows.Next() {
		var (
			id, userID        int64
			email, name, role string
			since             string
		)
		if err := rows.Scan(&id, &userID, &email, &name, &role, &since); err != nil {
			log.Println("failed to scan staff member:", err)
			continue
		}
		out = append(out, gin.H{
			"id": id, "user_id": userID, "email": email, "name": name,
			"role": role, "since": since,
			// Sent so the app can hide what somebody cannot use, rather than let
			// them find out by tapping it and being refused.
			"can": middleware.RoleCapabilities(middleware.Role(role)),
		})
	}

	c.JSON(http.StatusOK, gin.H{"data": out})
}

// AddStaff gives somebody a login for this business.
//
// POST /api/v1/companies/:companyId/staff
//
// Two cases, deliberately: an email nobody has used becomes a new account the owner sets
// a password for, because a counter boy will not be signing himself up; an email that is
// already an Invo account is simply added, and the owner never gets to set a password on
// somebody else's login.
func (h *StaffHandler) AddStaff(c *gin.Context) {
	companyID, ok := h.company(c)
	if !ok {
		return
	}

	var req struct {
		Name     string `json:"name"`
		Email    string `json:"email"`
		Password string `json:"password"`
		Role     string `json:"role"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid input"})
		return
	}

	req.Email = normalizeEmail(req.Email)
	req.Name = strings.TrimSpace(req.Name)
	role := strings.TrimSpace(strings.ToLower(req.Role))

	if req.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Give them a name."})
		return
	}
	// Only the two roles that can be handed out. An owner is the person whose business
	// it is, and there is exactly one — making a second here would mean two people who
	// can remove each other.
	if role != string(middleware.RoleManager) && role != string(middleware.RoleStaff) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Choose manager or staff."})
		return
	}

	tx, err := h.db.Begin()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to add them"})
		return
	}
	defer tx.Rollback()

	var userID int64
	err = tx.QueryRow(`SELECT id FROM users WHERE lower(email) = $1`, req.Email).Scan(&userID)

	switch {
	case err == sql.ErrNoRows:
		// A new account, made by the owner. Verified on the spot: the OTP at signup
		// proves an email belongs to the person typing it, and here the owner is
		// vouching for them in person — posting a code to an address the owner made
		// up would only lock the account nobody can open.
		if len(req.Password) < 8 {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Give them a password of at least 8 characters.",
			})
			return
		}
		hashed, hashErr := utils.HashPassword(req.Password)
		if hashErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to add them"})
			return
		}
		if err := tx.QueryRow(`
			INSERT INTO users (email, password_hash, is_verified)
			VALUES ($1, $2, TRUE)
			RETURNING id
		`, req.Email, hashed).Scan(&userID); err != nil {
			log.Println("failed to create staff user:", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to add them"})
			return
		}

	case err != nil:
		log.Println("failed to look up staff email:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to add them"})
		return

	default:
		// The email is already an Invo account. They are added to the business, and
		// their password is theirs — a password sent with this request is ignored
		// rather than applied, because setting one here would be a way to take over
		// any account whose address you can guess.
		req.Password = ""
	}

	var memberID int64
	err = tx.QueryRow(`
		INSERT INTO company_members (company_id, user_id, role, name, added_by)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (company_id, user_id) DO NOTHING
		RETURNING id
	`, companyID, userID, role, req.Name, c.GetInt("user_id")).Scan(&memberID)

	if err == sql.ErrNoRows {
		c.JSON(http.StatusConflict, gin.H{"error": "They already work here."})
		return
	}
	if err != nil {
		log.Println("failed to add staff member:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to add them"})
		return
	}

	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to add them"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message":   "Added",
		"member_id": memberID,
		"user_id":   userID,
	})
}

// UpdateStaff changes what somebody may do.
//
// PUT /api/v1/companies/:companyId/staff/:memberId
func (h *StaffHandler) UpdateStaff(c *gin.Context) {
	companyID, ok := h.company(c)
	if !ok {
		return
	}
	memberID, err := strconv.ParseInt(c.Param("memberId"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid member"})
		return
	}

	var req struct {
		Name string `json:"name"`
		Role string `json:"role"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid input"})
		return
	}
	role := strings.TrimSpace(strings.ToLower(req.Role))
	if role != string(middleware.RoleManager) && role != string(middleware.RoleStaff) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Choose manager or staff."})
		return
	}

	// Scoped to this company and never the owner's row: the owner cannot be demoted,
	// which would otherwise leave a business nobody can administer.
	result, err := h.db.Exec(`
		UPDATE company_members
		SET role = $1,
		    name = COALESCE(NULLIF($2, ''), name),
		    updated_at = NOW()
		WHERE id = $3 AND company_id = $4 AND role <> 'owner'
	`, role, strings.TrimSpace(req.Name), memberID, companyID)
	if err != nil {
		log.Println("failed to update staff member:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save that"})
		return
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "That person isn't on this shop's staff."})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Saved"})
}

// RemoveStaff takes somebody's access away.
//
// DELETE /api/v1/companies/:companyId/staff/:memberId
//
// This deletes the membership and nothing else. The user row stays, and it matters that
// it does: invoices, clients and items carry the id of whoever created them, and those
// foreign keys cascade. Deleting the person would delete the work — every invoice the
// counter boy wrote would go with him. Access ends immediately either way, on their very
// next request.
func (h *StaffHandler) RemoveStaff(c *gin.Context) {
	companyID, ok := h.company(c)
	if !ok {
		return
	}
	memberID, err := strconv.ParseInt(c.Param("memberId"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid member"})
		return
	}

	result, err := h.db.Exec(`
		DELETE FROM company_members
		WHERE id = $1 AND company_id = $2 AND role <> 'owner'
	`, memberID, companyID)
	if err != nil {
		log.Println("failed to remove staff member:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to remove them"})
		return
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "That person isn't on this shop's staff."})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Removed"})
}
