package middleware_test

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"invo-server/internal/middleware"

	"github.com/gin-gonic/gin"
	_ "github.com/lib/pq"
)

// A slow query is cut off by the request's deadline rather than running to completion
// while the caller waits — the whole point of giving the queries a context.
func TestDeadlineCancelsSlowQuery(t *testing.T) {
	db, err := sql.Open("postgres", "host=localhost port=5432 dbname=invo_android_test sslmode=disable")
	if err != nil {
		t.Skip("no database:", err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		t.Skip("no database:", err)
	}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.RequestDeadline(300 * time.Millisecond))
	r.GET("/slow", func(c *gin.Context) {
		// Ten seconds of sleeping in the database.
		var out string
		err := db.QueryRowContext(c.Request.Context(), `SELECT pg_sleep(10)::text`).Scan(&out)
		if err != nil {
			c.JSON(http.StatusGatewayTimeout, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	start := time.Now()
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/slow", nil))
	elapsed := time.Since(start)

	if elapsed > 3*time.Second {
		t.Fatalf("query ran for %s; the deadline did not cut it off", elapsed)
	}
	if w.Code != http.StatusGatewayTimeout {
		t.Fatalf("expected the handler to see a cancelled query, got %d", w.Code)
	}
	if ctxErr := context.Canceled; ctxErr == nil {
		t.Fatal("unreachable")
	}
	t.Logf("cut off after %s with %d", elapsed, w.Code)
}
