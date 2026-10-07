package handlers

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"time"

	database "invo-server/internal/db"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

const refreshTokenLifetime = 90 * 24 * time.Hour

// generateRefreshToken returns a random URL-safe token and its SHA-256 hex hash.
// The token is sent to the client; only the hash is stored in the database.
func generateRefreshToken() (token, hash string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return
	}
	token = base64.URLEncoding.EncodeToString(b)
	sum := sha256.Sum256([]byte(token))
	hash = hex.EncodeToString(sum[:])
	return
}

func hashRefreshToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// createSession inserts a session row and returns the session ID and plain-text refresh token.
// Called during login and OTP/email verification flows.
func createSession(
	c *gin.Context,
	db *sql.DB,
	userID int,
	deviceName, platform string,
) (sessionID, refreshToken string, err error) {
	var hash string
	refreshToken, hash, err = generateRefreshToken()
	if err != nil {
		return
	}

	ip := c.ClientIP()
	if ip == "::1" {
		ip = "127.0.0.1"
	}

	expiresAt := time.Now().Add(refreshTokenLifetime)
	err = db.QueryRowContext(c.Request.Context(), `
		INSERT INTO sessions (user_id, refresh_token_hash, device_name, platform, ip_address, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id
	`, userID, hash, deviceName, platform, ip, expiresAt).Scan(&sessionID)
	return
}

type SessionHandler struct {
	db        *database.Database
	jwtSecret []byte
	tokenExp  time.Duration
}

func NewSessionHandler(db *database.Database, jwtSecret []byte, tokenExp time.Duration) *SessionHandler {
	return &SessionHandler{db: db, jwtSecret: jwtSecret, tokenExp: tokenExp}
}

// POST /auth/token/refresh — exchange a refresh token for a new access + refresh token pair.
// Public endpoint — access token may already be expired when this is called.
func (h *SessionHandler) RefreshTokens(c *gin.Context) {
	var req struct {
		RefreshToken string `json:"refresh_token" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "refresh_token required"})
		return
	}

	hash := hashRefreshToken(req.RefreshToken)

	var sessionID string
	var userID int
	var email string
	err := h.db.DB.QueryRowContext(c.Request.Context(), `
		SELECT s.id, s.user_id, u.email
		FROM sessions s
		JOIN users u ON u.id = s.user_id
		WHERE s.refresh_token_hash = $1 AND s.expires_at > NOW()
	`, hash).Scan(&sessionID, &userID, &email)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid or expired refresh token"})
		return
	}

	// Rotate: replace the refresh token so the old one is dead immediately.
	newRefreshToken, newHash, err := generateRefreshToken()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Token generation failed"})
		return
	}

	_, err = h.db.DB.ExecContext(c.Request.Context(), `
		UPDATE sessions
		SET refresh_token_hash = $1, last_seen = NOW(), expires_at = $2
		WHERE id = $3
	`, newHash, time.Now().Add(refreshTokenLifetime), sessionID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Session update failed"})
		return
	}

	// Issue a new access token.
	now := time.Now()
	claims := jwt.MapClaims{
		"user_id": userID,
		"email":   email,
		"iat":     now.Unix(),
		"exp":     now.Add(h.tokenExp).Unix(),
		"sv":      sessionVersion(c.Request.Context(), h.db.DB, int64(userID)),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	accessToken, err := tok.SignedString(h.jwtSecret)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Token generation failed"})
		return
	}

	setSessionCookie(c, accessToken, h.tokenExp)

	c.JSON(http.StatusOK, gin.H{
		"token":         accessToken,
		"refresh_token": newRefreshToken,
		"session_id":    sessionID,
		"expires_in":    h.tokenExp.Seconds(),
		"token_type":    "Bearer",
	})
}

// GET /auth/sessions — list all active sessions for the authenticated user.
func (h *SessionHandler) ListSessions(c *gin.Context) {
	userID := c.GetInt("user_id")

	rows, err := h.db.DB.QueryContext(c.Request.Context(), `
		SELECT id, device_name, platform, ip_address, last_seen, created_at
		FROM sessions
		WHERE user_id = $1 AND expires_at > NOW()
		ORDER BY last_seen DESC
	`, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch sessions"})
		return
	}
	defer rows.Close()

	type Session struct {
		ID         string    `json:"id"`
		DeviceName string    `json:"device_name"`
		Platform   string    `json:"platform"`
		IPAddress  string    `json:"ip_address"`
		LastSeen   time.Time `json:"last_seen"`
		CreatedAt  time.Time `json:"created_at"`
	}

	sessions := make([]Session, 0)
	for rows.Next() {
		var s Session
		if scanErr := rows.Scan(&s.ID, &s.DeviceName, &s.Platform, &s.IPAddress, &s.LastSeen, &s.CreatedAt); scanErr != nil {
			continue
		}
		sessions = append(sessions, s)
	}

	c.JSON(http.StatusOK, gin.H{"sessions": sessions})
}

// DELETE /auth/sessions/:id — revoke a single session.
func (h *SessionHandler) RevokeSession(c *gin.Context) {
	userID := c.GetInt("user_id")
	sessionID := c.Param("id")

	res, err := h.db.DB.ExecContext(c.Request.Context(), `
		DELETE FROM sessions WHERE id = $1 AND user_id = $2
	`, sessionID, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to revoke session"})
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Session not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Session revoked"})
}

// DELETE /auth/sessions — revoke all sessions except the one in ?keep=<session_id>.
func (h *SessionHandler) RevokeAllOtherSessions(c *gin.Context) {
	userID := c.GetInt("user_id")
	keepID := c.Query("keep")

	var err error
	if keepID != "" {
		_, err = h.db.DB.ExecContext(c.Request.Context(), `
			DELETE FROM sessions WHERE user_id = $1 AND id != $2
		`, userID, keepID)
	} else {
		_, err = h.db.DB.ExecContext(c.Request.Context(), `
			DELETE FROM sessions WHERE user_id = $1
		`, userID)
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to revoke sessions"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Sessions revoked"})
}
