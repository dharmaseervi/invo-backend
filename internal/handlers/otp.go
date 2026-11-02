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
		Email string `json:"email" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Valid email required"})
		return
	}
	req.Email = normalizeEmail(req.Email)
	if !validEmail(req.Email) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Enter a valid email address"})
		return
	}

	// Check user exists
	var userExists bool
	h.db.DB.QueryRowContext(c.Request.Context(),
		`SELECT EXISTS(SELECT 1 FROM users WHERE lower(email) = $1)`,
		req.Email,
	).Scan(&userExists)

	if !userExists {
		c.JSON(http.StatusNotFound, gin.H{"error": "No account found with this email"})
		return
	}

	// Invalidate old OTPs
	h.db.DB.ExecContext(c.Request.Context(),
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
	codeHash, err := hashOneTimeCode(code)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate OTP"})
		return
	}

	_, err = h.db.DB.ExecContext(c.Request.Context(),
		`INSERT INTO otp_codes (email, code, expires_at) VALUES ($1, $2, $3)`,
		req.Email, codeHash, expiresAt,
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
		Email string `json:"email" binding:"required"`
		Code  string `json:"code" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Email and OTP required"})
		return
	}
	req.Email = normalizeEmail(req.Email)
	if !validEmail(req.Email) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Enter a valid email address"})
		return
	}

	// Codes are stored hashed, so verification compares against the newest live row
	// and burns it after too many wrong guesses.
	if err := consumeOneTimeCode(c.Request.Context(),
		h.db.DB, "otp_codes", req.Email, req.Code); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid or expired OTP"})
		return
	}
	var err error

	// Get user. is_verified is required here for the same reason password login
	// requires it — otherwise OTP is a way around email verification entirely.
	var userID int
	var email string
	var isVerified bool
	err = h.db.DB.QueryRowContext(c.Request.Context(),
		`SELECT id, email, COALESCE(is_verified, false) FROM users WHERE lower(email) = $1`,
		req.Email,
	).Scan(&userID, &email, &isVerified)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid or expired OTP"})
		return
	}
	if !isVerified {
		c.JSON(http.StatusForbidden, gin.H{"error": "Please verify your email address first"})
		return
	}

	resp, err := issueSession(c, h.db.DB, userID, "Unknown device", "unknown", "", h.jwtSecret, 24*time.Hour)
	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to create session; please sign in again"})
		return
	}
	setSessionCookie(c, resp["token"].(string), 24*time.Hour)
	resp["user"] = gin.H{"id": userID, "email": email}
	c.JSON(200, resp)
}
