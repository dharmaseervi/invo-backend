package middleware

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
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
		// The header first, which is how both phone apps authenticate. The website
		// uses a cookie the browser will not hand to JavaScript — see SessionCookie —
		// so a script injected into the page cannot read the session out of it, which
		// is exactly what localStorage allowed.
		var tokenString string
		if authHeader := c.GetHeader("Authorization"); authHeader != "" {
			parts := strings.Split(authHeader, " ")
			if len(parts) != 2 || parts[0] != "Bearer" {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid authorization format"})
				c.Abort()
				return
			}
			tokenString = parts[1]
		} else if cookie, err := c.Cookie(SessionCookie); err == nil && cookie != "" {
			// A cookie travels on its own, so a request carrying one must also prove it
			// came from this site rather than from a page somebody was tricked into
			// opening. SameSite=Strict does that for every browser in use, and the
			// check below refuses anything that arrives looking cross-site anyway.
			if !sameSiteRequest(c) {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
				c.Abort()
				return
			}
			tokenString = cookie
		}

		if tokenString == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authorization header missing"})
			c.Abort()
			return
		}

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
		var currentVersion int
		switch err := db.QueryRow(
			`SELECT tokens_valid_from, session_version FROM users WHERE id = $1`, userID,
		).Scan(&validFrom, &currentVersion); {
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

		// Revocation by counter where the token carries one.
		//
		// The timestamp rule cannot be made exact: iat is whole seconds, so a token
		// minted in the same second as the cutoff has to be honoured or a fresh login
		// could reject itself — which left a one-second window in which a token stolen
		// just before a logout still worked. Logout and password reset now raise
		// session_version, and a token carrying a different number is refused outright.
		//
		// Tokens issued before this claim existed are still on people's phones and have
		// no "sv", so they fall back to the timestamp rule until they expire.
		if sv, ok := claims["sv"].(float64); ok {
			if int(sv) != currentVersion {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "Session ended, please sign in again"})
				c.Abort()
				return
			}
		} else if int64(issuedAt) < validFrom.Unix() {
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

// retention is how long an idle key must be kept before forgetting it is safe.
//
// Dropping a key hands its owner a full burst again, so a key may only be forgotten
// once its bucket would have refilled anyway. With the email quota — 8 an hour, burst
// of 4 — a fixed ten-minute sweep was a way around the limit: wait eleven minutes,
// get four more messages, which is roughly twenty an hour rather than eight. Fast
// limiters still expire quickly, because their buckets refill in seconds.
func (l *keyedRateLimiter) retention() time.Duration {
	const floor = 10 * time.Minute
	if l.r <= 0 {
		return floor
	}
	refill := time.Duration(float64(l.burst) / float64(l.r) * float64(time.Second))
	if refill < floor {
		return floor
	}
	return refill
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

// cleanupStale forgets keys that have been idle long enough that their bucket would
// have refilled anyway, so a long-running server does not accumulate one entry per
// address or account forever — without giving anyone a fresh budget by waiting.
func (l *keyedRateLimiter) cleanupStale() {
	for {
		time.Sleep(5 * time.Minute)
		keep := l.retention()
		l.mu.Lock()
		for key, v := range l.visitors {
			if time.Since(v.lastSeen) > keep {
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

// UserRateLimiter throttles authenticated traffic per user account rather than per IP.
//
// Keying authenticated routes on the IP is wrong in two directions. Several staff in
// one shop share a NAT address, so they would share a single budget and throttle each
// other; and the limit has to be loose enough for a screen that fires a handful of
// requests at once, which makes it useless as a per-client ceiling. The account is the
// thing worth limiting, and it is already established by the auth middleware.
//
// Must be registered after AuthMiddleware, which is what sets user_id.
func UserRateLimiter(perSecond float64, burst int) gin.HandlerFunc {
	limiter := newKeyedRateLimiter(rate.Limit(perSecond), burst)

	return func(c *gin.Context) {
		key := c.ClientIP()
		if userID := c.GetInt("user_id"); userID != 0 {
			key = "user:" + strconv.Itoa(userID)
		}

		if !limiter.getLimiter(key).Allow() {
			c.JSON(http.StatusTooManyRequests, gin.H{"error": "Too many requests, please slow down"})
			c.Abort()
			return
		}
		c.Next()
	}
}

// EmailQuota caps how many account emails one address can be made to receive.
//
// The per-minute credential limit stops guessing, but it still allows a few hundred
// messages a day at one address: anybody can ask for a login code, a verification code
// or a password reset for an email they do not own, and every request sends a real
// message. That is a nuisance for the person receiving them and a way to burn the
// sending domain's reputation, which ends with genuine invoices going to spam.
//
// Keyed on the target address and on the caller's IP, so neither one address nor one
// source can be used to flood. An hourly budget with a small burst leaves room for
// somebody who mistypes their address, loses the first code, then asks again.
//
// The two budgets are set separately on purpose: an address only ever belongs to one
// person, but an IP is shared — the staff of one shop come from a single address, and
// giving them one address's budget between them would lock out the second person to
// ask for a code.
func EmailQuota(accountPerHour float64, accountBurst int, ipPerHour float64, ipBurst int) gin.HandlerFunc {
	byAccount := newKeyedRateLimiter(rate.Limit(accountPerHour/3600), accountBurst)
	byIP := newKeyedRateLimiter(rate.Limit(ipPerHour/3600), ipBurst)

	return func(c *gin.Context) {
		tooMany := func() {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error": "Too many codes requested. Wait a few minutes, and check your spam folder for the last one.",
			})
			c.Abort()
		}

		if !byIP.getLimiter("mail-ip:" + c.ClientIP()).Allow() {
			tooMany()
			return
		}

		// The body has to be restored: the handler binds it after this runs.
		body, err := io.ReadAll(io.LimitReader(c.Request.Body, maxCredentialBodyPeek))
		if err == nil {
			c.Request.Body = io.NopCloser(bytes.NewReader(body))

			var payload struct {
				Email string `json:"email"`
			}
			if json.Unmarshal(body, &payload) == nil && payload.Email != "" {
				key := "mail:" + strings.ToLower(strings.TrimSpace(payload.Email))
				if !byAccount.getLimiter(key).Allow() {
					tooMany()
					return
				}
			}
		}

		c.Next()
	}
}

// SessionCookie is the name of the website's session cookie.
//
// The web app used to keep its token in localStorage, where any script running on the
// page can read it — an injected script, a compromised dependency, a browser extension.
// The app escapes everything it renders and never touches innerHTML, so there was no
// known way in; "no known way in" is a weaker thing to rely on than a browser refusing
// to hand the value over at all.
//
// The Go server hosts both the website and the API on one origin, so it can set a cookie
// the page cannot read. The phone apps are unaffected: they send a header, which is
// still checked first.
const SessionCookie = "invo_session"

// sameSiteRequest reports whether a cookie-authenticated request plausibly came from
// this site's own pages.
//
// Cookies ride along automatically, which is what makes them worth having and also what
// makes cross-site request forgery possible. SameSite=Strict on the cookie is the real
// defence; this is the belt to that pair of braces, and it costs one header comparison.
//
// Sec-Fetch-Site cannot be set by page JavaScript, so when it is there it is worth
// believing. When it is absent the request is allowed through on the strength of
// SameSite alone: Safari did not send this header until 16.4, and refusing without it
// would have logged those browsers out on every request — a login that succeeds and
// then immediately fails, with nothing on screen to explain it. SameSite=Strict has
// been enforced far longer and is the defence that actually matters here; this header
// only catches a browser that somehow sent the cookie anyway.
func sameSiteRequest(c *gin.Context) bool {
	switch c.GetHeader("Sec-Fetch-Site") {
	case "cross-site":
		return false
	default:
		// same-origin, same-site, "none" (a typed address or a bookmark), or a browser
		// too old to say.
		return true
	}
}
