package handlers

import (
	"database/sql"
	"fmt"
	database "invo-server/internal/db"
	"invo-server/internal/models"
	"invo-server/internal/services"
	utils "invo-server/internal/util"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

type AuthHandler struct {
	db              *database.Database
	jwtSecret       []byte
	emailService    *services.EmailService // ← add this
	tokenExpiration time.Duration
}

// NewAuthHandler creates a new authentication handler
func NewAuthHandler(db *database.Database, jwtSecret []byte, emailService *services.EmailService) *AuthHandler {
	return &AuthHandler{
		db:              db,
		jwtSecret:       jwtSecret,
		emailService:    emailService,
		tokenExpiration: 24 * time.Hour, // Default 24 hour expiration
	}
}

// Register handles user registration
func (h *AuthHandler) Register(c *gin.Context) {
	var user models.UserRegister

	if err := c.ShouldBindJSON(&user); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid input format"})
		return
	}

	if err := user.Validate(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Check if user already exists
	var exists bool
	err := h.db.DB.QueryRow(
		"SELECT EXISTS(SELECT 1 FROM users WHERE email = $1)",
		user.Email,
	).Scan(&exists)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}
	if exists {
		c.JSON(http.StatusConflict, gin.H{"error": "Email already registered"})
		return
	}

	// Hash password
	hashedPassword, err := utils.HashPassword(user.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Password processing failed"})
		return
	}

	// Insert user — is_verified = FALSE by default
	var id int
	err = h.db.DB.QueryRow(`
        INSERT INTO users (email, password_hash, is_verified)
        VALUES ($1, $2, FALSE)
        RETURNING id`,
		user.Email, hashedPassword,
	).Scan(&id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "User creation failed"})
		return
	}

	// Generate and send OTP
	code, err := generateOTP()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate OTP"})
		return
	}

	expiresAt := time.Now().Add(10 * time.Minute)
	_, err = h.db.DB.Exec(
		`INSERT INTO otp_codes (email, code, expires_at) VALUES ($1, $2, $3)`,
		user.Email, code, expiresAt,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save OTP"})
		return
	}

	// Send verification email
	err = h.emailService.SendVerificationEmail(user.Email, code)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to send verification email"})
		return
	}
	log.Println("DB error:", err)
	c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})

	// Return user ID and email — NO token yet (not verified)
	c.JSON(http.StatusCreated, gin.H{
		"message":               "Account created! Check your email for verification code.",
		"user_id":               id,
		"email":                 user.Email,
		"requires_verification": true,
	})
}

// Login handles user authentication and JWT generation
func (h *AuthHandler) Login(c *gin.Context) {
	var login models.UserLogin
	if err := c.ShouldBindJSON(&login); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid login data"})
		return
	}

	// Step 1 — Get user from database FIRST
	var user models.User
	err := h.db.DB.QueryRow(`
        SELECT id, email, password_hash 
        FROM users 
        WHERE email = $1`,
		login.Email,
	).Scan(&user.ID, &user.Email, &user.PasswordHash)

	// Step 2 — Check if user exists
	if err == sql.ErrNoRows {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid credentials"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Login process failed"})
		return
	}

	// Step 3 — Verify password
	if !utils.CheckPasswordHash(login.Password, user.PasswordHash) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid credentials"})
		return
	}

	// Step 4 — Check if email is verified (AFTER confirming user exists)
	var isVerified bool
	h.db.DB.QueryRow(
		`SELECT is_verified FROM users WHERE email = $1`, login.Email,
	).Scan(&isVerified)

	if !isVerified {
		c.JSON(http.StatusForbidden, gin.H{
			"error":                 "Email not verified. Please check your email for the verification code.",
			"requires_verification": true,
			"email":                 login.Email,
		})
		return
	}

	// Step 5 — Generate JWT
	now := time.Now()
	claims := jwt.MapClaims{
		"user_id": user.ID,
		"email":   user.Email,
		"iat":     now.Unix(),
		"exp":     now.Add(h.tokenExpiration).Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(h.jwtSecret)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Token generation failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"user": gin.H{
			"id":    user.ID,
			"email": user.Email,
		},
		"token":      tokenString,
		"expires_in": h.tokenExpiration.Seconds(),
		"token_type": "Bearer",
	})
}

// POST /api/v1/verify-email
func (h *AuthHandler) VerifyEmail(c *gin.Context) {
	var req struct {
		Email string `json:"email" binding:"required,email"`
		Code  string `json:"code" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Email and code required"})
		return
	}

	// Check OTP
	var otpID int
	var expiresAt time.Time
	err := h.db.DB.QueryRow(`
        SELECT id, expires_at FROM otp_codes
        WHERE email = $1 AND code = $2 AND used = FALSE
        ORDER BY created_at DESC LIMIT 1
    `, req.Email, req.Code).Scan(&otpID, &expiresAt)

	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid OTP"})
		return
	}

	if time.Now().After(expiresAt) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "OTP has expired"})
		return
	}

	// Mark OTP used
	h.db.DB.Exec(`UPDATE otp_codes SET used = TRUE WHERE id = $1`, otpID)

	// Mark user as verified
	_, err = h.db.DB.Exec(
		`UPDATE users SET is_verified = TRUE WHERE email = $1`,
		req.Email,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to verify account"})
		return
	}

	// Get user details
	var userID int
	h.db.DB.QueryRow(
		`SELECT id FROM users WHERE email = $1`, req.Email,
	).Scan(&userID)

	// Generate JWT — now verified ✅
	now := time.Now()
	claims := jwt.MapClaims{
		"user_id": userID,
		"email":   req.Email,
		"iat":     now.Unix(),
		"exp":     now.Add(h.tokenExpiration).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(h.jwtSecret)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Token generation failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Email verified successfully!",
		"user": gin.H{
			"id":    userID,
			"email": req.Email,
		},
		"token":      tokenString,
		"expires_in": h.tokenExpiration.Seconds(),
		"token_type": "Bearer",
	})
}

// POST /api/v1/resend-verification
func (h *AuthHandler) ResendVerification(c *gin.Context) {
	var req struct {
		Email string `json:"email" binding:"required,email"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Email required"})
		return
	}

	// Check user exists and not verified
	var isVerified bool
	err := h.db.DB.QueryRow(
		`SELECT is_verified FROM users WHERE email = $1`, req.Email,
	).Scan(&isVerified)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Account not found"})
		return
	}
	if isVerified {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Account already verified"})
		return
	}

	// Invalidate old OTPs
	h.db.DB.Exec(
		`UPDATE otp_codes SET used = TRUE WHERE email = $1 AND used = FALSE`,
		req.Email,
	)

	// Generate new OTP
	code, _ := generateOTP()
	expiresAt := time.Now().Add(10 * time.Minute)
	h.db.DB.Exec(
		`INSERT INTO otp_codes (email, code, expires_at) VALUES ($1, $2, $3)`,
		req.Email, code, expiresAt,
	)

	h.emailService.SendVerificationEmail(req.Email, code)

	c.JSON(http.StatusOK, gin.H{"message": "Verification code resent"})
}

// RefreshToken generates a new token for valid users
func (h *AuthHandler) RefreshToken(c *gin.Context) {
	// Get user ID from context (set by auth middleware)
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	// Generate new token
	now := time.Now()
	claims := jwt.MapClaims{
		"user_id": userID,
		"iat":     now.Unix(),
		"exp":     now.Add(h.tokenExpiration).Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(h.jwtSecret)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Token refresh failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"token":      tokenString,
		"expires_in": h.tokenExpiration.Seconds(),
		"token_type": "Bearer",
	})
}

// Logout endpoint (optional - useful for client-side cleanup)
func (h *AuthHandler) Logout(c *gin.Context) {
	// Since JWT is stateless, server-side logout isn't needed
	// However, we can return instructions for the client
	c.JSON(http.StatusOK, gin.H{
		"message":      "Successfully logged out",
		"instructions": "Please remove the token from your client storage",
	})
}

// DELETE /api/v1/account
func (h *AuthHandler) DeleteAccount(c *gin.Context) {
	userIDVal, exists := c.Get("user_id")
	authHeader := c.GetHeader("Authorization")
	log.Printf("🔑 Auth header received: %s", authHeader)

	log.Printf("👤 user_id exists: %v, value: %v", exists, userIDVal)

	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	userID := userIDVal.(int)
	log.Printf("🗑️ Deleting account for user ID: %d", userID)

	queries := []struct {
		name  string
		query string
	}{
		{"otp_codes", `DELETE FROM otp_codes WHERE email = (SELECT email FROM users WHERE id = $1)`},
		{"invoice_items", `DELETE FROM invoice_items WHERE invoice_id IN (SELECT id FROM invoices WHERE company_id IN (SELECT id FROM companies WHERE user_id = $1))`},
		{"invoice_addresses", `DELETE FROM invoice_addresses WHERE invoice_id IN (SELECT id FROM invoices WHERE company_id IN (SELECT id FROM companies WHERE user_id = $1))`},
		{"payments", `DELETE FROM payments WHERE company_id IN (SELECT id FROM companies WHERE user_id = $1)`},
		{"ledger", `DELETE FROM ledger_entries WHERE company_id IN (SELECT id FROM companies WHERE user_id = $1)`},
		{"credit_notes", `DELETE FROM credit_notes WHERE company_id IN (SELECT id FROM companies WHERE user_id = $1)`},
		{"invoices", `DELETE FROM invoices WHERE company_id IN (SELECT id FROM companies WHERE user_id = $1)`},
		{"clients", `DELETE FROM clients WHERE company_id IN (SELECT id FROM companies WHERE user_id = $1)`},
		{"items", `DELETE FROM items WHERE company_id IN (SELECT id FROM companies WHERE user_id = $1)`},
		{"categories", `DELETE FROM categories WHERE company_id IN (SELECT id FROM companies WHERE user_id = $1)`},
		{"expenses", `DELETE FROM expensess WHERE company_id IN (SELECT id FROM companies WHERE user_id = $1)`},
		{"company_addresses", `DELETE FROM company_addresses WHERE company_id IN (SELECT id FROM companies WHERE user_id = $1)`},
		{"company_banks", `DELETE FROM company_bank_accounts WHERE company_id IN (SELECT id FROM companies WHERE user_id = $1)`},
		{"companies", `DELETE FROM companies WHERE user_id = $1`},
		{"users", `DELETE FROM users WHERE id = $1`},
	}

	for _, q := range queries {
		if _, err := h.db.DB.Exec(q.query, userID); err != nil {
			log.Printf("❌ Failed deleting %s: %v", q.name, err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": fmt.Sprintf("Failed at: %s — %v", q.name, err),
			})
			return
		}
		log.Printf("✅ Deleted %s for user %d", q.name, userID)
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Account deleted successfully",
	})
}

// POST /api/v1/forgot-password
func (h *AuthHandler) ForgotPassword(c *gin.Context) {
	var req struct {
		Email string `json:"email" binding:"required,email"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Valid email required"})
		return
	}

	// Check user exists
	var exists bool
	h.db.DB.QueryRow(
		`SELECT EXISTS(SELECT 1 FROM users WHERE email = $1)`, req.Email,
	).Scan(&exists)

	// Always return success (don't reveal if email exists)
	if !exists {
		c.JSON(http.StatusOK, gin.H{"message": "If this email exists, a reset code has been sent"})
		return
	}

	// Invalidate old tokens
	h.db.DB.Exec(
		`UPDATE password_reset_tokens SET used = TRUE WHERE email = $1 AND used = FALSE`,
		req.Email,
	)

	// Generate OTP
	code, err := generateOTP()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate code"})
		return
	}

	expiresAt := time.Now().Add(10 * time.Minute)
	_, err = h.db.DB.Exec(
		`INSERT INTO password_reset_tokens (email, code, expires_at) VALUES ($1, $2, $3)`,
		req.Email, code, expiresAt,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save reset code"})
		return
	}

	// Send email
	err = h.emailService.SendPasswordResetEmail(req.Email, code)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to send email"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":    "Reset code sent to " + req.Email,
		"expires_in": "10 minutes",
	})
}

// POST /api/v1/reset-password
func (h *AuthHandler) ResetPassword(c *gin.Context) {
	var req struct {
		Email       string `json:"email" binding:"required,email"`
		Code        string `json:"code" binding:"required"`
		NewPassword string `json:"new_password" binding:"required,min=6"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Check OTP
	var tokenID int
	var expiresAt time.Time
	err := h.db.DB.QueryRow(`
        SELECT id, expires_at FROM password_reset_tokens
        WHERE email = $1 AND code = $2 AND used = FALSE
        ORDER BY created_at DESC LIMIT 1
    `, req.Email, req.Code).Scan(&tokenID, &expiresAt)

	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid or expired code"})
		return
	}

	if time.Now().After(expiresAt) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Reset code has expired"})
		return
	}

	// Hash new password
	hashedPassword, err := utils.HashPassword(req.NewPassword)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to process password"})
		return
	}

	// Update password
	_, err = h.db.DB.Exec(
		`UPDATE users SET password_hash = $1 WHERE email = $2`,
		hashedPassword, req.Email,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update password"})
		return
	}

	// Mark token as used
	h.db.DB.Exec(`UPDATE password_reset_tokens SET used = TRUE WHERE id = $1`, tokenID)

	// Generate JWT — log them in automatically
	var userID int
	h.db.DB.QueryRow(`SELECT id FROM users WHERE email = $1`, req.Email).Scan(&userID)

	now := time.Now()
	claims := jwt.MapClaims{
		"user_id": userID,
		"email":   req.Email,
		"iat":     now.Unix(),
		"exp":     now.Add(h.tokenExpiration).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(h.jwtSecret)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate token"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Password reset successfully",
		"user": gin.H{
			"id":    userID,
			"email": req.Email,
		},
		"token":      tokenString,
		"expires_in": h.tokenExpiration.Seconds(),
		"token_type": "Bearer",
	})
}
