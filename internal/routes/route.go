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
	expenseHandler := handlers.NewExpenseHandler(db) // ← Add this line
	clientAddressHandler := handlers.NewClientAddressHandler(db)
	companyAddressHandler := handlers.NewCompanyAddressHandler(db)
	invoicePDFHandler := handlers.NewInvoicePDFHandler(db)

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

		// Company routes
		protected.POST("/companies", companyHandler.CreateCompany)
		protected.GET("/companies", companyHandler.GetMyCompanies)
		protected.GET("/companies/:companyId/address", companyAddressHandler.GetCompanyAddress)
		protected.POST("/companies/:companyId/address", companyAddressHandler.SaveCompanyAddress)

		// Client routes
		protected.POST("/clients", clientHandler.CreateClient)
		protected.GET("/companies/:companyId/clients", clientHandler.GetClients)
		protected.GET("/clients/:clientId/address", clientAddressHandler.GetClientAddress)
		protected.POST("/clients/:clientId/address", clientAddressHandler.SaveClientAddress)

		// Item routes
		protected.POST("/items", itemHandler.CreateItem)
		protected.GET("/items/:companyId/all", itemHandler.GetItems)
		protected.GET("/item/:itemId/one", itemHandler.GetItemByID)

		// Category routes
		protected.POST("/categories", categoryHandler.CreateCategory)
		protected.GET("/categories/:companyId", categoryHandler.GetCategories)

		// Invoice routes
		protected.POST("/invoices", invoiceHandler.CreateInvoice)
		protected.GET("/invoices", invoiceHandler.GetInvoices)
		protected.GET("/invoices/:id", invoiceHandler.GetInvoiceByID)
		protected.GET("/invoices/number-preview", invoiceHandler.GetInvoiceNumberPreview)

		// Expense routes ← Add these lines
		protected.POST("/expenses", expenseHandler.CreateExpense)
		protected.GET("/expenses/:id", expenseHandler.GetExpenseByID)
		protected.PUT("/expenses/:id", expenseHandler.UpdateExpense)
		protected.DELETE("/expenses/:id", expenseHandler.DeleteExpense)
		protected.GET("/companies/:companyId/expenses", expenseHandler.GetExpenses)
		// protected.GET("/companies/:id/expenses/range", expenseHandler.GetExpensesByDateRange)
		// protected.GET("/companies/:id/expenses/stats", expenseHandler.GetExpenseStats)

		protected.POST("/invoices/:id/generate-pdf", invoicePDFHandler.GenerateInvoicePDF)

	}
}
