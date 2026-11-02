package handlers

import (
	"database/sql"
	database "invo-server/internal/db"
	"invo-server/internal/middleware"
	"invo-server/internal/models"
	"invo-server/internal/services"
	utils "invo-server/internal/util"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
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
		tokenExpiration: 7 * 24 * time.Hour,
	}
}

// Register handles user registration
func (h *AuthHandler) Register(c *gin.Context) {
	var user models.UserRegister

	if err := c.ShouldBindJSON(&user); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid input format"})
		return
	}
	user.Email = normalizeEmail(user.Email)

	if err := user.Validate(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Check if user already exists
	var exists bool
	err := h.db.DB.QueryRowContext(c.Request.Context(),
		"SELECT EXISTS(SELECT 1 FROM users WHERE lower(email) = $1)",
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
	err = h.db.DB.QueryRowContext(c.Request.Context(),
		`
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
	codeHash, err := hashOneTimeCode(code)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate OTP"})
		return
	}
	_, err = h.db.DB.ExecContext(c.Request.Context(),
		`INSERT INTO otp_codes (email, code, expires_at) VALUES ($1, $2, $3)`,
		user.Email, codeHash, expiresAt,
	)
	if err != nil {
		log.Printf("OTP insert error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save OTP"})
		return // ← ADD THIS!
	}

	// Send verification email.
	//
	// A failure here must not fail the signup. The account row is already written, so
	// returning 500 left the user with an account they could neither verify nor
	// re-register — retrying just returned "Email already registered", a dead end with
	// no way forward. Signup succeeds, the client is told the code did not go out, and
	// the existing resend endpoint covers the retry.
	emailSent := true
	if err = h.emailService.SendVerificationEmail(user.Email, code); err != nil {
		log.Printf("Email error: %v", err)
		emailSent = false
	}

	message := "Account created! Check your email for verification code."
	if !emailSent {
		message = "Account created, but we couldn't send the verification code. Tap resend to try again."
	}

	// ✅ Only ONE response at the end
	c.JSON(http.StatusCreated, gin.H{
		"message":               message,
		"user_id":               id,
		"email":                 user.Email,
		"requires_verification": true,
		"email_sent":            emailSent,
	})
}

// Login handles user authentication and JWT generation
func (h *AuthHandler) Login(c *gin.Context) {
	var login struct {
		Email      string `json:"email" binding:"required"`
		Password   string `json:"password" binding:"required"`
		DeviceName string `json:"device_name"`
		Platform   string `json:"platform"`
	}
	if err := c.ShouldBindJSON(&login); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid login data"})
		return
	}
	login.Email = normalizeEmail(login.Email)
	if login.DeviceName == "" {
		login.DeviceName = "Unknown device"
	}
	if login.Platform == "" {
		login.Platform = "ios"
	}

	// Step 1 — Get user from database FIRST
	var user models.User
	err := h.db.DB.QueryRowContext(c.Request.Context(),
		`
        SELECT id, email, password_hash 
        FROM users 
        WHERE lower(email) = $1`,
		login.Email,
	).Scan(&user.ID, &user.Email, &user.PasswordHash)

	// Step 2 — Check if user exists
	if err == sql.ErrNoRows {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid credentials"})
		return
	}
	if err != nil {
		log.Println("login: failed to load user:", err)
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
	h.db.DB.QueryRowContext(c.Request.Context(),
		`SELECT is_verified FROM users WHERE lower(email) = $1`, login.Email,
	).Scan(&isVerified)

	if !isVerified {
		c.JSON(http.StatusForbidden, gin.H{
			"error":                 "Email not verified. Please check your email for the verification code.",
			"requires_verification": true,
			"email":                 login.Email,
		})
		return
	}

	resp, err := issueSession(c, h.db.DB, user.ID, login.DeviceName, login.Platform, user.PasswordHash, h.jwtSecret, h.tokenExpiration)
	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to create session; please sign in again"})
		return
	}
	setSessionCookie(c, resp["token"].(string), h.tokenExpiration)
	resp["user"] = gin.H{"id": user.ID, "email": user.Email}
	c.JSON(200, resp)
}

// POST /api/v1/verify-email
func (h *AuthHandler) VerifyEmail(c *gin.Context) {
	var req struct {
		Email string `json:"email" binding:"required"`
		Code  string `json:"code" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Email and code required"})
		return
	}
	req.Email = normalizeEmail(req.Email)
	if !validEmail(req.Email) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Enter a valid email address"})
		return
	}

	// Check OTP
	if err := consumeOneTimeCode(c.Request.Context(),
		h.db.DB, "otp_codes", req.Email, req.Code); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid or expired OTP"})
		return
	}
	var err error

	// Mark user as verified
	_, err = h.db.DB.ExecContext(c.Request.Context(),
		`UPDATE users SET is_verified = TRUE WHERE lower(email) = $1`,
		req.Email,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to verify account"})
		return
	}

	// Get user details
	var userID int
	h.db.DB.QueryRowContext(c.Request.Context(),
		`SELECT id FROM users WHERE lower(email) = $1`, req.Email,
	).Scan(&userID)

	resp, err := issueSession(c, h.db.DB, userID, "Unknown device", "unknown", "", h.jwtSecret, h.tokenExpiration)
	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to create session; please sign in again"})
		return
	}
	setSessionCookie(c, resp["token"].(string), h.tokenExpiration)
	resp["user"] = gin.H{"id": userID, "email": req.Email}
	resp["message"] = "Email verified successfully!"
	c.JSON(200, resp)
}

// POST /api/v1/resend-verification
func (h *AuthHandler) ResendVerification(c *gin.Context) {
	var req struct {
		Email string `json:"email" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Email required"})
		return
	}
	req.Email = normalizeEmail(req.Email)
	if !validEmail(req.Email) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Enter a valid email address"})
		return
	}

	// Check user exists and not verified
	var isVerified bool
	err := h.db.DB.QueryRowContext(c.Request.Context(),
		`SELECT is_verified FROM users WHERE lower(email) = $1`, req.Email,
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
	h.db.DB.ExecContext(c.Request.Context(),
		`UPDATE otp_codes SET used = TRUE WHERE email = $1 AND used = FALSE`,
		req.Email,
	)

	// Generate new OTP
	code, _ := generateOTP()
	expiresAt := time.Now().Add(10 * time.Minute)
	resendHash, hashErr := hashOneTimeCode(code)
	if hashErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate OTP"})
		return
	}
	h.db.DB.ExecContext(c.Request.Context(),
		`INSERT INTO otp_codes (email, code, expires_at) VALUES ($1, $2, $3)`,
		req.Email, resendHash, expiresAt,
	)

	h.emailService.SendVerificationEmail(req.Email, code)

	c.JSON(http.StatusOK, gin.H{"message": "Verification code resent"})
}

// RefreshToken generates a new token for valid users
// RefreshToken keeps the older cookie/client endpoint compatible. It rechecks
// revocation under the same lock used by logout and never creates a replacement
// for a session that was revoked after middleware authorized the request.
func (h *AuthHandler) RefreshToken(c *gin.Context) {
	tx, err := h.db.DB.BeginTx(c.Request.Context(), nil)
	if err != nil {
		c.JSON(500, gin.H{"error": "Token refresh failed"})
		return
	}
	defer tx.Rollback()
	userID := c.GetInt("user_id")
	var version int
	var email string
	err = tx.QueryRowContext(c.Request.Context(), `SELECT session_version,email FROM users WHERE id=$1 FOR UPDATE`, userID).Scan(&version, &email)
	if err != nil || !requestSessionStillValid(c, tx, userID, version) {
		c.JSON(401, gin.H{"error": "Session ended, please sign in again"})
		return
	}
	var resp gin.H
	if sid := c.GetString("session_id"); sid != "" {
		var token string
		token, err = signSessionToken(userID, version, email, sid, h.jwtSecret, h.tokenExpiration)
		resp = gin.H{"token": token, "session_id": sid, "expires_in": h.tokenExpiration.Seconds(), "token_type": "Bearer"}
	} else {
		// Upgrade a still-valid pre-sid client at its next refresh.
		resp, err = sessionCredentials(c, tx, userID, version, email, "Existing device", "unknown", h.jwtSecret, h.tokenExpiration)
	}
	if err != nil {
		c.JSON(500, gin.H{"error": "Token refresh failed"})
		return
	}
	if err = tx.Commit(); err != nil {
		c.JSON(500, gin.H{"error": "Token refresh failed"})
		return
	}
	setSessionCookie(c, resp["token"].(string), h.tokenExpiration)
	c.JSON(200, resp)
}

// Logout preserves the established all-device logout behavior. Both access and
// refresh credentials are invalidated atomically, including concurrent rotation.
func (h *AuthHandler) Logout(c *gin.Context) {
	tx, err := h.db.DB.BeginTx(c.Request.Context(), nil)
	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to log out"})
		return
	}
	defer tx.Rollback()
	userID := c.GetInt("user_id")
	_, err = tx.ExecContext(c.Request.Context(), `UPDATE users SET tokens_valid_from=NOW(),session_version=session_version+1,legacy_tokens_revoked=TRUE WHERE id=$1`, userID)
	if err == nil {
		_, err = tx.ExecContext(c.Request.Context(), `DELETE FROM sessions WHERE user_id=$1`, userID)
	}
	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to log out"})
		return
	}
	if err = tx.Commit(); err != nil {
		c.JSON(500, gin.H{"error": "Failed to log out"})
		return
	}
	clearSessionCookie(c)
	c.JSON(200, gin.H{"message": "Signed out of all devices"})
}

// DELETE /api/v1/account
func (h *AuthHandler) DeleteAccount(c *gin.Context) {
	userIDVal, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	userID := userIDVal.(int)

	// Deleting everything is the most destructive thing this API does, and a valid
	// bearer token was the only thing standing in front of it — so a token lifted from
	// a phone was enough to wipe a business's books. Re-entering the password proves
	// the person holding the token is the owner.
	//
	// Only enforced when the caller sends one, until both apps can: the iPhone app in
	// the store today sends nothing, and refusing it there would break the deletion
	// route that Apple and Google require. So a password that is sent must be right,
	// and REQUIRE_REAUTH=true makes it compulsory for everyone once the iOS update that
	// asks for it is out. Set that, and a caller that omits the password is refused.
	if err := confirmAccountPassword(c, h.db.DB, userID); err != nil {
		return
	}

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

		// Before the user row goes, hand over whatever they made in somebody else's
		// shop. invoices, clients, items, estimates and expenses all carry the id of
		// whoever created them, and every one of those foreign keys cascades — so
		// deleting a counter boy's account would take that shop's invoices with it,
		// silently, from a business that was never his. They pass to the owner, which
		// is who they were always for. The deletions above have already removed this
		// user's own businesses, so nothing left here belongs to them.
		{"handover_invoices", `UPDATE invoices i SET user_id = c.user_id
			FROM companies c WHERE c.id = i.company_id AND i.user_id = $1`},
		{"handover_clients", `UPDATE clients cl SET user_id = c.user_id
			FROM companies c WHERE c.id = cl.company_id AND cl.user_id = $1`},
		{"handover_items", `UPDATE items it SET user_id = c.user_id
			FROM companies c WHERE c.id = it.company_id AND it.user_id = $1`},
		{"handover_estimates", `UPDATE estimates e SET user_id = c.user_id
			FROM companies c WHERE c.id = e.company_id AND e.user_id = $1`},
		{"handover_expenses", `UPDATE expensess ex SET user_id = c.user_id
			FROM companies c WHERE c.id = ex.company_id AND ex.user_id = $1`},

		{"users", `DELETE FROM users WHERE id = $1`},
	}

	tx, err := h.db.DB.BeginTx(c.Request.Context(), nil)
	if err != nil {
		log.Printf("❌ Failed to start account deletion transaction: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete account"})
		return
	}

	for _, q := range queries {
		if _, err := tx.ExecContext(c.Request.Context(),
			q.query, userID); err != nil {
			tx.Rollback()
			log.Printf("❌ Failed deleting %s for user %d: %v", q.name, userID, err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete account"})
			return
		}
	}

	if err := tx.Commit(); err != nil {
		log.Printf("❌ Failed to commit account deletion for user %d: %v", userID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete account"})
		return
	}

	log.Printf("✅ Account deleted for user %d", userID)

	c.JSON(http.StatusOK, gin.H{
		"message": "Account deleted successfully",
	})
}

// POST /api/v1/forgot-password
func (h *AuthHandler) ForgotPassword(c *gin.Context) {
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
	var exists bool
	h.db.DB.QueryRowContext(c.Request.Context(),
		`SELECT EXISTS(SELECT 1 FROM users WHERE lower(email) = $1)`, req.Email,
	).Scan(&exists)

	// Always return success (don't reveal if email exists)
	if !exists {
		c.JSON(http.StatusOK, gin.H{"message": "If this email exists, a reset code has been sent"})
		return
	}

	// Invalidate old tokens
	h.db.DB.ExecContext(c.Request.Context(),
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
	codeHash, err := hashOneTimeCode(code)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save reset code"})
		return
	}

	_, err = h.db.DB.ExecContext(c.Request.Context(),
		`INSERT INTO password_reset_tokens (email, code, expires_at) VALUES ($1, $2, $3)`,
		req.Email, codeHash, expiresAt,
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
		Email       string `json:"email" binding:"required"`
		Code        string `json:"code" binding:"required"`
		NewPassword string `json:"new_password" binding:"required,min=6"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	req.Email = normalizeEmail(req.Email)
	if !validEmail(req.Email) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Enter a valid email address"})
		return
	}

	// Reset codes are stored hashed and burned after too many wrong guesses, the same
	// as login OTPs — this is the path an attacker would grind to take over an account.
	if err := consumeOneTimeCode(c.Request.Context(),
		h.db.DB, "password_reset_tokens", req.Email, req.Code); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid or expired code"})
		return
	}
	var err error

	// Hash new password
	hashedPassword, err := utils.HashPassword(req.NewPassword)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to process password"})
		return
	}

	tx, err := h.db.DB.BeginTx(c.Request.Context(), nil)
	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to update password"})
		return
	}
	defer tx.Rollback()
	var userID, version int
	err = tx.QueryRowContext(c.Request.Context(), `UPDATE users SET password_hash=$1,tokens_valid_from=NOW(),session_version=session_version+1,legacy_tokens_revoked=TRUE WHERE lower(email)=$2 RETURNING id,session_version`, hashedPassword, req.Email).Scan(&userID, &version)
	if err == nil {
		_, err = tx.ExecContext(c.Request.Context(), `DELETE FROM sessions WHERE user_id=$1`, userID)
	}
	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to update password"})
		return
	}
	resp, err := sessionCredentials(c, tx, userID, version, req.Email, "Password reset", "unknown", h.jwtSecret, h.tokenExpiration)
	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to create session"})
		return
	}
	if err = tx.Commit(); err != nil {
		c.JSON(500, gin.H{"error": "Failed to update password"})
		return
	}
	setSessionCookie(c, resp["token"].(string), h.tokenExpiration)
	resp["user"] = gin.H{"id": userID, "email": req.Email}
	resp["message"] = "Password reset successfully"
	c.JSON(200, resp)
}

// setSessionCookie gives the browser a session it cannot read from JavaScript.
//
// HttpOnly is the point of the exercise. Secure keeps it off plaintext connections, and
// is skipped only when the server is itself running without TLS, which is development —
// a cookie marked Secure is simply never sent over http:// and the website would appear
// to log in and then immediately be logged out.
//
// SameSite=Strict is what makes a cookie safe to authenticate with: the browser will not
// attach it to a request started by any other site, so a page that tries to post to this
// API on a logged-in person's behalf gets an anonymous request. The website and the API
// are one origin in production, so nothing legitimate is lost.
func setSessionCookie(c *gin.Context, token string, lifetime time.Duration) {
	secure := c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https"

	http.SetCookie(c.Writer, &http.Cookie{
		Name:     middleware.SessionCookie,
		Value:    token,
		Path:     "/",
		MaxAge:   int(lifetime.Seconds()),
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteStrictMode,
	})
}

// clearSessionCookie ends the browser's side of a session.
func clearSessionCookie(c *gin.Context) {
	secure := c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https"

	http.SetCookie(c.Writer, &http.Cookie{
		Name:     middleware.SessionCookie,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteStrictMode,
	})
}
