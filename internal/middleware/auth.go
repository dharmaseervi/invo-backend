package middleware

import (
	"database/sql"
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

// ipRateLimiter gives every client IP its own token bucket. A single shared
// limiter (the old behavior) means one busy or abusive IP throttles every
// other user of the API; per-IP limiting only punishes the IP that earns it.
type ipRateLimiter struct {
	mu       sync.Mutex
	visitors map[string]*visitor
	r        rate.Limit
	burst    int
}

func newIPRateLimiter(r rate.Limit, burst int) *ipRateLimiter {
	l := &ipRateLimiter{
		visitors: make(map[string]*visitor),
		r:        r,
		burst:    burst,
	}
	go l.cleanupStale()
	return l
}

func (l *ipRateLimiter) getLimiter(ip string) *rate.Limiter {
	l.mu.Lock()
	defer l.mu.Unlock()

	v, exists := l.visitors[ip]
	if !exists {
		limiter := rate.NewLimiter(l.r, l.burst)
		l.visitors[ip] = &visitor{limiter: limiter, lastSeen: time.Now()}
		return limiter
	}
	v.lastSeen = time.Now()
	return v.limiter
}

// cleanupStale drops any IP that hasn't made a request in 10 minutes, so a
// long-running server doesn't accumulate one entry per IP forever.
func (l *ipRateLimiter) cleanupStale() {
	for {
		time.Sleep(5 * time.Minute)
		l.mu.Lock()
		for ip, v := range l.visitors {
			if time.Since(v.lastSeen) > 10*time.Minute {
				delete(l.visitors, ip)
			}
		}
		l.mu.Unlock()
	}
}

// RateLimiter middleware to prevent brute force attacks — 5 requests/sec per
// client IP with a burst of 10, tracked independently per IP.
func RateLimiter() gin.HandlerFunc {
	limiter := newIPRateLimiter(rate.Limit(5), 10)
	return func(c *gin.Context) {
		if !limiter.getLimiter(c.ClientIP()).Allow() {
			c.JSON(http.StatusTooManyRequests, gin.H{"error": "Too many requests"})
			c.Abort()
			return
		}
		c.Next()
	}
}
