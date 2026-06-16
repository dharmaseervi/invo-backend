package handlers

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"net/http"
	"time"

	database "invo-server/internal/db"
	"invo-server/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

type OTPHandler struct {
	db           *database.Database
	emailService *services.EmailService
	jwtSecret    []byte
}

func NewOTPHandler(
	db *database.Database,
	emailService *services.EmailService,
	jwtSecret []byte,
) *OTPHandler {
	return &OTPHandler{
		db:           db,
		emailService: emailService,
		jwtSecret:    jwtSecret,
	}
}

func generateOTP() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

// POST /api/v1/send-otp
func (h *OTPHandler) SendOTP(c *gin.Context) {
	var req struct {
		Email string `json:"email" binding:"required,email"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Valid email required"})
		return
	}

	// Check user exists
	var userExists bool
	h.db.DB.QueryRow(
		`SELECT EXISTS(SELECT 1 FROM users WHERE email = $1)`,
		req.Email,
	).Scan(&userExists)

	if !userExists {
		c.JSON(http.StatusNotFound, gin.H{"error": "No account found with this email"})
		return
	}

	// Invalidate old OTPs
	h.db.DB.Exec(
		`UPDATE otp_codes SET used = TRUE WHERE email = $1 AND used = FALSE`,
		req.Email,
	)

	// Generate OTP
	code, err := generateOTP()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate OTP"})
		return
	}

	// Save to DB
	expiresAt := time.Now().Add(10 * time.Minute)
	_, err = h.db.DB.Exec(
		`INSERT INTO otp_codes (email, code, expires_at) VALUES ($1, $2, $3)`,
		req.Email, code, expiresAt,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save OTP"})
		return
	}

	// Send email
	err = h.emailService.SendOTPEmail(req.Email, code)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to send OTP email"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":    "OTP sent to " + req.Email,
		"expires_in": "10 minutes",
	})
}

// POST /api/v1/verify-otp
func (h *OTPHandler) VerifyOTP(c *gin.Context) {
	var req struct {
		Email string `json:"email" binding:"required,email"`
		Code  string `json:"code" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Email and OTP required"})
		return
	}

	// Find valid OTP
	var otpID int
	var expiresAt time.Time
	err := h.db.DB.QueryRow(`
		SELECT id, expires_at FROM otp_codes
		WHERE email = $1 AND code = $2 AND used = FALSE
		ORDER BY created_at DESC
		LIMIT 1
	`, req.Email, req.Code).Scan(&otpID, &expiresAt)

	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid OTP"})
		return
	}

	// Check expiry
	if time.Now().After(expiresAt) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "OTP has expired"})
		return
	}

	// Mark as used
	h.db.DB.Exec(`UPDATE otp_codes SET used = TRUE WHERE id = $1`, otpID)

	// Get user
	var userID int
	var email string
	err = h.db.DB.QueryRow(
		`SELECT id, email FROM users WHERE email = $1`,
		req.Email,
	).Scan(&userID, &email)
	if err != nil {

		c.JSON(http.StatusInternalServerError, gin.H{"error": "User not found"})
		return
	}

	// Generate JWT — same as your existing Login handler
	now := time.Now()
	claims := jwt.MapClaims{
		"user_id": userID,
		"email":   email,
		"iat":     now.Unix(),
		"exp":     now.Add(24 * time.Hour).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(h.jwtSecret)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Token generation failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"user": gin.H{
			"id":    userID,
			"email": email,
		},
		"token":      tokenString,
		"expires_in": 86400,
		"token_type": "Bearer",
	})
}
