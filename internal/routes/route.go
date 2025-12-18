package routes

import (
	"invo-server/internal/config"
	database "invo-server/internal/db"
	"invo-server/internal/handlers"
	"invo-server/internal/middleware"
	"net/http"

	"github.com/gin-gonic/gin"
)

func RegisterRoutes(r *gin.Engine, db *database.Database, cfg *config.Config) {

	authHandler := handlers.NewAuthHandler(db, []byte(cfg.JWT.Secret))
	userHandler := handlers.NewUserHandler(db)
	companyHandler := handlers.NewCompanyHandler(db)
	clientHandler := handlers.NewClientHandler(db)
	itemHandler := handlers.NewItemHandler(db)
	categoryHandler := handlers.NewCategoryHandler(db)
	invoiceHandler := handlers.NewInvoiceHandler(db)

	// Health check
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// Public routes
	public := r.Group("/api/v1")
	{
		public.POST("/register", authHandler.Register)
		public.POST("/login", authHandler.Login)
	}

	// Protected routes
	protected := r.Group("/api/v1")
	protected.Use(middleware.AuthMiddleware([]byte(cfg.JWT.Secret)))
	{
		protected.POST("/refresh-token", authHandler.RefreshToken)
		protected.POST("/logout", authHandler.Logout)
		protected.GET("/profile", userHandler.GetUserProfile)
		protected.POST("/companies", companyHandler.CreateCompany)
		protected.GET("/companies", companyHandler.GetMyCompanies)
		protected.POST("/clients", clientHandler.CreateClient)
		protected.GET("/companies/:id/clients", clientHandler.GetClients)
		protected.POST("/items", itemHandler.CreateItem)
		protected.GET("/items/:companyId/all", itemHandler.GetItems)
		protected.GET("/item/:itemId/one", itemHandler.GetItemByID)
		protected.POST("/categories", categoryHandler.CreateCategory)
		protected.GET("/categories/:companyId", categoryHandler.GetCategories)

		protected.POST("/invoices", invoiceHandler.CreateInvoice)
		protected.GET("/invoices", invoiceHandler.GetInvoices)
		protected.GET("/invoices/:id", invoiceHandler.GetInvoiceByID)

	}
}
