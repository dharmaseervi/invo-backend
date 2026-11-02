package handlers

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	database "invo-server/internal/db"
	"strings"
)

const refreshTokenLifetime = 90 * 24 * time.Hour

func generateRefreshToken() (token, hash string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return
	}
	token = base64.URLEncoding.EncodeToString(b)
	hash = hashRefreshToken(token)
	return
}
func hashRefreshToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// Every credential mutation locks users first, then sessions. This makes refresh,
// reset, logout and device revocation serializable without a lock-order deadlock.
// Tokens are signed before committing; a failed session write never signs a user in.
func sessionCredentials(c *gin.Context, tx *sql.Tx, userID, version int, email, device, platform string, secret []byte, lifetime time.Duration) (gin.H, error) {
	refresh, hash, err := generateRefreshToken()
	if err != nil {
		return nil, err
	}
	if device == "" {
		device = "Unknown device"
	}
	if platform == "" {
		platform = "unknown"
	}
	var sid string
	err = tx.QueryRowContext(c.Request.Context(), `INSERT INTO sessions
 (user_id, session_version, refresh_token_hash, device_name, platform, ip_address, expires_at)
 VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id`, userID, version, hash, device, platform, c.ClientIP(), time.Now().Add(refreshTokenLifetime)).Scan(&sid)
	if err != nil {
		return nil, err
	}
	access, err := signSessionToken(userID, version, email, sid, secret, lifetime)
	if err != nil {
		return nil, err
	}
	return gin.H{"token": access, "refresh_token": refresh, "session_id": sid, "expires_in": lifetime.Seconds(), "token_type": "Bearer"}, nil
}
func signSessionToken(userID, version int, email, sid string, secret []byte, lifetime time.Duration) (string, error) {
	now := time.Now()
	return jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"user_id": userID, "email": email, "sv": version, "sid": sid, "iat": now.Unix(), "exp": now.Add(lifetime).Unix()}).SignedString(secret)
}

func issueSession(c *gin.Context, db *sql.DB, userID int, device, platform, expectedPasswordHash string, secret []byte, lifetime time.Duration) (gin.H, error) {
	tx, err := db.BeginTx(c.Request.Context(), nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var version int
	var email, passwordHash string
	err = tx.QueryRowContext(c.Request.Context(), `SELECT session_version,email,password_hash FROM users WHERE id=$1 FOR UPDATE`, userID).Scan(&version, &email, &passwordHash)
	if err != nil {
		return nil, err
	}
	// A reset between password validation and this lock must not authenticate the old password.
	if expectedPasswordHash != "" && expectedPasswordHash != passwordHash {
		return nil, errors.New("credentials changed; sign in again")
	}
	resp, err := sessionCredentials(c, tx, userID, version, email, device, platform, secret, lifetime)
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return resp, nil
}

type SessionHandler struct {
	db        *database.Database
	jwtSecret []byte
	tokenExp  time.Duration
}

func NewSessionHandler(db *database.Database, jwtSecret []byte, tokenExp time.Duration) *SessionHandler {
	return &SessionHandler{db: db, jwtSecret: jwtSecret, tokenExp: tokenExp}
}

func (h *SessionHandler) RefreshTokens(c *gin.Context) {
	var req struct {
		RefreshToken string `json:"refresh_token" binding:"required,max=256"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "refresh_token required"})
		return
	}
	hash := hashRefreshToken(req.RefreshToken)
	tx, err := h.db.DB.BeginTx(c.Request.Context(), nil)
	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to refresh session"})
		return
	}
	defer tx.Rollback()
	var userID, version int
	var email string
	// Look up the user before locking the session; a concurrent rotation/delete is
	// checked again below after obtaining the user lock.
	err = tx.QueryRowContext(c.Request.Context(), `SELECT u.id,u.email,u.session_version FROM users u WHERE u.id=(SELECT user_id FROM sessions WHERE refresh_token_hash=$1) FOR UPDATE`, hash).Scan(&userID, &email, &version)
	if err != nil {
		c.JSON(401, gin.H{"error": "Invalid or expired refresh token"})
		return
	}
	refresh, newHash, err := generateRefreshToken()
	if err != nil {
		c.JSON(500, gin.H{"error": "Token generation failed"})
		return
	}
	var sid string
	err = tx.QueryRowContext(c.Request.Context(), `UPDATE sessions SET refresh_token_hash=$1,last_seen=NOW(),expires_at=$2 WHERE refresh_token_hash=$3 AND user_id=$4 AND session_version=$5 AND expires_at>NOW() RETURNING id`, newHash, time.Now().Add(refreshTokenLifetime), hash, userID, version).Scan(&sid)
	if err != nil {
		c.JSON(401, gin.H{"error": "Invalid or expired refresh token"})
		return
	}
	access, err := signSessionToken(userID, version, email, sid, h.jwtSecret, h.tokenExp)
	if err != nil {
		c.JSON(500, gin.H{"error": "Token generation failed"})
		return
	}
	if err = tx.Commit(); err != nil {
		c.JSON(500, gin.H{"error": "Failed to refresh session"})
		return
	}
	setSessionCookie(c, access, h.tokenExp)
	c.JSON(200, gin.H{"token": access, "refresh_token": refresh, "session_id": sid, "expires_in": h.tokenExp.Seconds(), "token_type": "Bearer"})
}

func (h *SessionHandler) ListSessions(c *gin.Context) {
	rows, err := h.db.DB.QueryContext(c.Request.Context(), `SELECT s.id,s.device_name,s.platform,s.ip_address,s.last_seen,s.created_at FROM sessions s JOIN users u ON u.id=s.user_id WHERE s.user_id=$1 AND s.expires_at>NOW() AND s.session_version=u.session_version ORDER BY s.last_seen DESC`, c.GetInt("user_id"))
	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to fetch sessions"})
		return
	}
	defer rows.Close()
	type session struct {
		ID         string    `json:"id"`
		DeviceName string    `json:"device_name"`
		Platform   string    `json:"platform"`
		IPAddress  string    `json:"ip_address"`
		LastSeen   time.Time `json:"last_seen"`
		CreatedAt  time.Time `json:"created_at"`
	}
	sessions := []session{}
	for rows.Next() {
		var s session
		if err = rows.Scan(&s.ID, &s.DeviceName, &s.Platform, &s.IPAddress, &s.LastSeen, &s.CreatedAt); err != nil {
			c.JSON(500, gin.H{"error": "Failed to read sessions"})
			return
		}
		sessions = append(sessions, s)
	}
	if rows.Err() != nil {
		c.JSON(500, gin.H{"error": "Failed to read sessions"})
		return
	}
	c.JSON(200, gin.H{"sessions": sessions, "current_session_id": c.GetString("session_id")})
}

func (h *SessionHandler) RevokeSession(c *gin.Context)          { h.revoke(c, false) }
func (h *SessionHandler) RevokeAllOtherSessions(c *gin.Context) { h.revoke(c, true) }
func (h *SessionHandler) revoke(c *gin.Context, others bool) {
	sid := c.Param("id")
	if others {
		sid = c.GetString("session_id")
		if sid == "" {
			c.JSON(401, gin.H{"error": "Sign in again to manage other devices"})
			return
		}
		if keep := c.Query("keep"); keep != "" && keep != sid {
			c.JSON(400, gin.H{"error": "Only the current session can be kept"})
			return
		}
	}
	if !validSessionID(sid) {
		c.JSON(400, gin.H{"error": "Invalid session id"})
		return
	}
	tx, err := h.db.DB.BeginTx(c.Request.Context(), nil)
	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to revoke sessions"})
		return
	}
	defer tx.Rollback()
	userID := c.GetInt("user_id")
	// Disable pre-sid access tokens when the user exercises device revocation; they
	// cannot be attributed to a device. Retained modern sessions remain valid.
	var version int
	err = tx.QueryRowContext(c.Request.Context(), `UPDATE users SET legacy_tokens_revoked=TRUE WHERE id=$1 RETURNING session_version`, userID).Scan(&version)
	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to revoke sessions"})
		return
	}
	if !requestSessionStillValid(c, tx, userID, version) {
		c.JSON(401, gin.H{"error": "Session ended, please sign in again"})
		return
	}
	query := `DELETE FROM sessions WHERE user_id=$1 AND id=$2`
	if others {
		query = `DELETE FROM sessions WHERE user_id=$1 AND id<>$2`
	}
	result, err := tx.ExecContext(c.Request.Context(), query, userID, sid)
	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to revoke sessions"})
		return
	}
	count, err := result.RowsAffected()
	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to revoke sessions"})
		return
	}
	if !others && count == 0 {
		c.JSON(404, gin.H{"error": "Session not found"})
		return
	}
	if err = tx.Commit(); err != nil {
		c.JSON(500, gin.H{"error": "Failed to revoke sessions"})
		return
	}
	if sid == c.GetString("session_id") && !others {
		clearSessionCookie(c)
	}
	c.JSON(http.StatusOK, gin.H{"message": "Sessions revoked"})
}

// Recheck after taking the user lock: middleware may have run before a reset or
// another revoke finished. Never allow an in-flight old request to regain authority.
func requestSessionStillValid(c *gin.Context, tx *sql.Tx, userID, version int) bool {
	if c.GetInt("session_version") != version {
		return false
	}
	sid := c.GetString("session_id")
	if sid == "" {
		var revoked bool
		err := tx.QueryRowContext(c.Request.Context(), `SELECT legacy_tokens_revoked FROM users WHERE id=$1`, userID).Scan(&revoked)
		return err == nil && !revoked
	}
	var exists bool
	err := tx.QueryRowContext(c.Request.Context(), `SELECT EXISTS(SELECT 1 FROM sessions WHERE id=$1 AND user_id=$2 AND session_version=$3 AND expires_at>NOW())`, sid, userID, version).Scan(&exists)
	return err == nil && exists
}

func validSessionID(id string) bool {
	if len(id) != 36 || id[8] != '-' || id[13] != '-' || id[18] != '-' || id[23] != '-' {
		return false
	}
	_, err := hex.DecodeString(strings.ReplaceAll(id, "-", ""))
	return err == nil
}
