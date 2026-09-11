package middleware

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"

	"github.com/golang-jwt/jwt/v5"
)

// AuthMiddleware verifies JWT tokens in incoming requests.
//
// db is used to honour token revocation: a JWT cannot be withdrawn once signed, so
// logout and password reset record a cutoff on the user and anything issued before it
// is refused here. That costs one primary-key lookup per request, which is the price
// of being able to invalidate a stolen token before it expires on its own.
func AuthMiddleware(jwtSecret []byte, db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get Authorization header
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authorization header missing"})
			c.Abort()
			return
		}

		// Check Bearer scheme
		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid authorization format"})
			c.Abort()
			return
		}

		tokenString := parts[1]

		// Parse and validate token
		token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
			// Validate signing method
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, jwt.ErrSignatureInvalid
			}
			return jwtSecret, nil
		})

		if err != nil {
			if err == jwt.ErrSignatureInvalid {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token signature"})
			} else {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid or expired token"})
			}
			c.Abort()
			return
		}

		// Extract and validate claims
		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok || !token.Valid {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token claims"})
			c.Abort()
			return
		}

		// Check token expiration
		if exp, ok := claims["exp"].(float64); ok {
			if time.Now().Unix() > int64(exp) {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "Token expired"})
				c.Abort()
				return
			}
		}

		// Set user information in context
		uidFloat, ok := claims["user_id"].(float64)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid user_id in token"})
			c.Abort()
			return
		}
		userID := int(uidFloat)
		c.Set("user_id", userID)

		email, ok := claims["email"].(string)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid email in token"})
			c.Abort()
			return
		}
		c.Set("email", email)

		issuedAt, ok := claims["iat"].(float64)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token"})
			c.Abort()
			return
		}

		var validFrom time.Time
		switch err := db.QueryRow(
			`SELECT tokens_valid_from FROM users WHERE id = $1`, userID,
		).Scan(&validFrom); {
		case err == sql.ErrNoRows:
			// The account was deleted while a token was still in circulation.
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token"})
			c.Abort()
			return
		case err != nil:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to verify session"})
			c.Abort()
			return
		}

		// Second granularity: iat is whole seconds, so a token minted in the same second
		// as the cutoff must still be honoured or a fresh login could reject itself.
		if int64(issuedAt) < validFrom.Unix() {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Session ended, please sign in again"})
			c.Abort()
			return
		}

		c.Next()
	}
}

// visitor tracks one client IP's limiter and when it was last seen, so stale
// entries can be swept instead of growing the map forever.
type visitor struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// keyedRateLimiter gives every key its own token bucket. A single shared limiter
// means one busy or abusive client throttles everyone; keying it only punishes the
// client that earns it. The key is the IP for general traffic, and the account for
// credential endpoints — limiting purely by IP lets an attacker spread an attack on
// one account across many addresses.
type keyedRateLimiter struct {
	mu       sync.Mutex
	visitors map[string]*visitor
	r        rate.Limit
	burst    int
}

func newKeyedRateLimiter(r rate.Limit, burst int) *keyedRateLimiter {
	l := &keyedRateLimiter{
		visitors: make(map[string]*visitor),
		r:        r,
		burst:    burst,
	}
	go l.cleanupStale()
	return l
}

func (l *keyedRateLimiter) getLimiter(key string) *rate.Limiter {
	l.mu.Lock()
	defer l.mu.Unlock()

	v, exists := l.visitors[key]
	if !exists {
		limiter := rate.NewLimiter(l.r, l.burst)
		l.visitors[key] = &visitor{limiter: limiter, lastSeen: time.Now()}
		return limiter
	}
	v.lastSeen = time.Now()
	return v.limiter
}

// cleanupStale drops any key idle for 10 minutes, so a long-running server does not
// accumulate one entry per address or account forever.
func (l *keyedRateLimiter) cleanupStale() {
	for {
		time.Sleep(5 * time.Minute)
		l.mu.Lock()
		for key, v := range l.visitors {
			if time.Since(v.lastSeen) > 10*time.Minute {
				delete(l.visitors, key)
			}
		}
		l.mu.Unlock()
	}
}

// RateLimiter middleware to prevent brute force attacks — 5 requests/sec per
// client IP with a burst of 10, tracked independently per IP.
func RateLimiter() gin.HandlerFunc {
	limiter := newKeyedRateLimiter(rate.Limit(5), 10)
	return func(c *gin.Context) {
		if !limiter.getLimiter(c.ClientIP()).Allow() {
			c.JSON(http.StatusTooManyRequests, gin.H{"error": "Too many requests"})
			c.Abort()
			return
		}
		c.Next()
	}
}


// maxCredentialBodyPeek caps how much of a request body is read to find the account
// being targeted. Auth payloads are tiny; anything larger is not worth buffering.
const maxCredentialBodyPeek = 8 << 10

// CredentialRateLimiter throttles credential endpoints per account as well as per IP.
//
// Per-IP limiting alone does not stop credential stuffing: an attacker with a pool of
// addresses gets the full per-IP budget from each one against the same account. Keying
// on the target email caps the attempts an account can receive no matter where they
// come from. Both limits apply — whichever runs out first.
func CredentialRateLimiter(perMinute float64, burst int) gin.HandlerFunc {
	byAccount := newKeyedRateLimiter(rate.Limit(perMinute/60), burst)
	byIP := newKeyedRateLimiter(rate.Limit(perMinute/60), burst)

	return func(c *gin.Context) {
		if !byIP.getLimiter(c.ClientIP()).Allow() {
			c.JSON(http.StatusTooManyRequests, gin.H{"error": "Too many attempts, please wait a moment"})
			c.Abort()
			return
		}

		// The body has to be restored: handlers bind it after this runs.
		body, err := io.ReadAll(io.LimitReader(c.Request.Body, maxCredentialBodyPeek))
		if err == nil {
			c.Request.Body = io.NopCloser(bytes.NewReader(body))

			var payload struct {
				Email string `json:"email"`
			}
			if json.Unmarshal(body, &payload) == nil && payload.Email != "" {
				key := strings.ToLower(strings.TrimSpace(payload.Email))
				if !byAccount.getLimiter(key).Allow() {
					c.JSON(http.StatusTooManyRequests, gin.H{"error": "Too many attempts, please wait a moment"})
					c.Abort()
					return
				}
			}
		}

		c.Next()
	}
}
