package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// The point of the per-account limit is that spreading an attack across many source
// addresses must not buy more attempts against one account.
func TestCredentialRateLimiterCapsPerAccountAcrossIPs(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(CredentialRateLimiter(60, 3)) // burst of 3
	r.POST("/login", func(c *gin.Context) { c.Status(http.StatusOK) })

	send := func(ip, email string) int {
		req := httptest.NewRequest(http.MethodPost, "/login",
			strings.NewReader(`{"email":"`+email+`","password":"x"}`))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = ip + ":12345"
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w.Code
	}

	// Each request comes from a different address, so only the account limit can stop it.
	for i, ip := range []string{"1.1.1.1", "2.2.2.2", "3.3.3.3"} {
		if code := send(ip, "victim@example.com"); code != http.StatusOK {
			t.Fatalf("attempt %d from %s: got %d, want 200", i+1, ip, code)
		}
	}

	if code := send("4.4.4.4", "victim@example.com"); code != http.StatusTooManyRequests {
		t.Fatalf("4th attempt from a fresh IP: got %d, want 429", code)
	}

	// A different account must be unaffected by the first one's exhausted budget.
	if code := send("5.5.5.5", "someone.else@example.com"); code != http.StatusOK {
		t.Fatalf("unrelated account: got %d, want 200", code)
	}
}

// The handler binds the body after the limiter reads it, so it must be restored.
func TestCredentialRateLimiterRestoresBody(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(CredentialRateLimiter(60, 5))
	r.POST("/login", func(c *gin.Context) {
		var body struct {
			Email string `json:"email"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			c.Status(http.StatusBadRequest)
			return
		}
		c.String(http.StatusOK, body.Email)
	})

	req := httptest.NewRequest(http.MethodPost, "/login",
		strings.NewReader(`{"email":"reader@example.com"}`))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "9.9.9.9:1234"
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK || w.Body.String() != "reader@example.com" {
		t.Fatalf("handler could not read body after limiter: code=%d body=%q", w.Code, w.Body.String())
	}
}
