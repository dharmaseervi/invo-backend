package main

import (
	"invo-server/internal/config"
	database "invo-server/internal/db"
	"invo-server/internal/routes"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

func main() {
	cfg := config.Load()

	db, err := database.NewDatabase(cfg.GetDSN())

	log.Println("🔄 Running database migrations... ", cfg.GetDbUrl())

	database.RunMigrations(cfg.GetDbUrl())

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
	r.Static("/screenshots", "./public/screenshots")
	// Health check for cron keep-alive
	r.GET("/api/v1/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})
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
		Addr:         addr,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
	}

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal("Server failed to start:", err)
	}
}
