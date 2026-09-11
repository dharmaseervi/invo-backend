package main

import (
	"context"
	"invo-server/internal/config"
	database "invo-server/internal/db"
	"invo-server/internal/routes"
	"log"
	"net/http"
	"os"
	"os/signal"
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

	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())

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
	log.Println("Server stopped")
}
