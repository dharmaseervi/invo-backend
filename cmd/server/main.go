package main

import (
	"context"
	"invo-server/internal/config"
	database "invo-server/internal/db"
	"invo-server/internal/observability"
	"invo-server/internal/routes"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"
)

// maxRequestBytes caps any single request body. Without a limit, one client can make
// the server buffer arbitrary amounts of memory. Invoice payloads are kilobytes.
const maxRequestBytes = 2 << 20 // 2 MiB

func main() {
	cfg := config.Load()

	db, err := database.NewDatabase(cfg.GetDSN())

	// Migrations are opt-out via RUN_MIGRATIONS=false.
	//
	// Running them on every boot is fine for a single instance but wrong for more than
	// one: replicas race to migrate the same database, and every cold start pays for
	// version-check round trips before it can serve a request. Defaults to true so
	// existing deployments are unaffected; turn it off once migrations run as their own
	// deploy step.
	//
	// Never log the DSN: it carries the database password, and application logs are
	// retained and readable far more widely than the credential itself should be.
	if os.Getenv("RUN_MIGRATIONS") != "false" {
		log.Println("🔄 Running database migrations...")
		database.RunMigrations(cfg.GetDbUrl())
	} else {
		log.Println("Skipping migrations (RUN_MIGRATIONS=false)")
	}

	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}
	defer db.DB.Close()

	if cfg.Environment == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	// Error reporting is opt-in via SENTRY_DSN and inert without it, so nothing here
	// requires an account or a third-party service to be reachable.
	if observability.Init(cfg.Environment, os.Getenv("RELEASE_VERSION")) {
		defer observability.Flush()
	}

	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())

	// After Recovery so a panic is still turned into a 500 for the caller; reporting
	// must not change what the client sees.
	r.Use(observability.Middleware())
	// Panics are only half the story: a handled 500 is the commoner failure and the
	// one worth waking up for.
	r.Use(observability.ReportServerErrors())

	// CORS Middleware — locked to an explicit allowlist (ALLOWED_ORIGINS, comma-separated).
	// The mobile app is unaffected: it's not a browser and never sends/needs an Origin header.
	// This only matters for a web client calling the API cross-origin.
	allowedOrigins := map[string]bool{}
	for _, origin := range strings.Split(os.Getenv("ALLOWED_ORIGINS"), ",") {
		origin = strings.TrimSpace(origin)
		if origin != "" {
			allowedOrigins[origin] = true
		}
	}

	r.Use(func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")
		if origin != "" && allowedOrigins[origin] {
			c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
			c.Writer.Header().Set("Vary", "Origin")
		}
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	})

	// Landing page
	r.GET("/", func(c *gin.Context) {
		c.File("./static/index.html")
	})
	r.GET("/privacy", func(c *gin.Context) {
		c.File("./static/privacy.html")
	})
	r.GET("/terms", func(c *gin.Context) {
		c.File("./static/terms.html")
	})
	r.Static("/screenshots", "./public/screenshots")

	// Web app, served from the same origin as the API so CORS never applies and there
	// is no second service to deploy or pay for.
	//
	// Next.js exports a directory per route (trailingSlash), so /app/invoices is really
	// static/app/invoices/index.html. Gin's Static cannot express that, hence the
	// explicit resolver below.
	const webRoot = "./static/app"

	serveWeb := func(c *gin.Context, requested string) {
		// Join through Clean and confirm the result is still inside webRoot: a request
		// for /app/../../.env would otherwise escape and serve whatever it names.
		target := filepath.Join(webRoot, filepath.Clean("/"+requested))
		if !strings.HasPrefix(target, filepath.Clean(webRoot)+string(os.PathSeparator)) &&
			target != filepath.Clean(webRoot) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Not found"})
			return
		}

		// A file, then the directory's index.html, then the export's own 404 page —
		// so a refresh deep in the app renders instead of falling through to the API.
		if info, err := os.Stat(target); err == nil && !info.IsDir() {
			c.File(target)
			return
		}
		if index := filepath.Join(target, "index.html"); fileExists(index) {
			c.File(index)
			return
		}
		if notFound := filepath.Join(webRoot, "404.html"); fileExists(notFound) {
			// Written directly rather than via c.File, which calls http.ServeFile and
			// overwrites the status with 200 — a missing page must not report success.
			if body, err := os.ReadFile(notFound); err == nil {
				c.Data(http.StatusNotFound, "text/html; charset=utf-8", body)
				return
			}
		}
		c.JSON(http.StatusNotFound, gin.H{"error": "Not found"})
	}

	// HEAD as well as GET: Next.js probes routes with HEAD when prefetching a link, and
	// a GET-only route answers those with 404 — which reads as a broken app to anything
	// that checks a page exists before fetching it.
	r.GET("/app", func(c *gin.Context) { serveWeb(c, "/") })
	r.HEAD("/app", func(c *gin.Context) { serveWeb(c, "/") })
	r.GET("/app/*filepath", func(c *gin.Context) { serveWeb(c, c.Param("filepath")) })
	r.HEAD("/app/*filepath", func(c *gin.Context) { serveWeb(c, c.Param("filepath")) })

	r.NoRoute(func(c *gin.Context) {
		c.JSON(http.StatusNotFound, gin.H{"error": "Not found"})
	})
	// Health check for cron keep-alive. It pings the database, because a server that
	// answers "ok" while its database is unreachable tells a load balancer to keep
	// sending traffic it cannot serve.
	r.GET("/api/v1/health", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()

		if err := db.DB.PingContext(ctx); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "degraded", "database": "unreachable"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok", "database": "ok"})
	})
	r.Use(func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxRequestBytes)
		c.Next()
	})

	// JSON list responses compress by roughly 70-80%. On a mobile connection that is a
	// far larger win than any query tuning, and it costs one line. Excludes the static
	// screenshot images, which are already compressed formats.
	r.Use(gzip.Gzip(gzip.DefaultCompression, gzip.WithExcludedPaths([]string{"/screenshots"})))

	// ✅ Register all routes (moved out)
	routes.RegisterRoutes(r, db, cfg)

	// Start server
	port := cfg.Server.Port
	if envPort := os.Getenv("PORT"); envPort != "" {
		port = envPort
	}

	addr := "0.0.0.0:" + port
	log.Printf("🚀 Server running on %s", addr)

	srv := &http.Server{
		Addr:    addr,
		Handler: r,
		// ReadHeaderTimeout specifically bounds slow header attacks, where a client
		// trickles headers to hold a connection open indefinitely.
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal("Server failed to start:", err)
		}
	}()

	// Wait for a termination signal, then stop accepting new connections and give
	// in-flight requests a chance to finish. Without this, a deploy kills requests
	// mid-write — including a transaction that has committed but not yet responded.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Println("Forced shutdown:", err)
	}

	// Anything queued is lost once the process exits, so give it a moment.
	observability.Flush()
	log.Println("Server stopped")
}

// fileExists reports whether a regular file is present at path.
func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
