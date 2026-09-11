package routes

import (
	"invo-server/internal/config"
	database "invo-server/internal/db"
	"invo-server/internal/handlers"
	"invo-server/internal/middleware"
	"invo-server/internal/services"
	"net/http"

	"github.com/gin-gonic/gin"
)

func RegisterRoutes(r *gin.Engine, db *database.Database, cfg *config.Config) {

	userHandler := handlers.NewUserHandler(db)
	companyHandler := handlers.NewCompanyHandler(db)
	clientHandler := handlers.NewClientHandler(db)
	itemHandler := handlers.NewItemHandler(db)
	categoryHandler := handlers.NewCategoryHandler(db)
	expenseHandler := handlers.NewExpenseHandler(db) // ← Add this line
	clientAddressHandler := handlers.NewClientAddressHandler(db)
	companyAddressHandler := handlers.NewCompanyAddressHandler(db)
	invoicePDFHandler := handlers.NewInvoicePDFHandler(db)
	dashboard := handlers.NewDashboardHandler(db)
	companyBankHandlerss := handlers.NewCompanyBankHandler(db.DB)
	gstReportHandler := handlers.NewGSTReportHandler(db.DB)
	agingReportHandler := handlers.NewAgingReportHandler(db.DB)
	stockReportHandler := handlers.NewStockReportHandler(db.DB)
	estimateHandler := handlers.NewEstimateHandler(db)
	pushService := services.NewPushService(db.DB, cfg)
	deviceTokenHandler := handlers.NewDeviceTokenHandler(pushService)
	services.StartOverdueChecker(db.DB, pushService, cfg.OverdueCheckInterval)

	ledgerService := services.NewLedgerService(db.DB)
	ledgerHandler := handlers.NewLedgerHandler(ledgerService, db.DB)
	invoiceHandler := handlers.NewInvoiceHandler(db, ledgerService, pushService)
	creditNoteService := services.NewCreditNoteService(db.DB, ledgerService)

	paymentService := services.NewPaymentService(db.DB, ledgerService)
	paymentHandler := handlers.NewPaymentHandler(db, paymentService)
	creditNoteHandler := handlers.NewCreditNoteHandler(creditNoteService, db.DB) // ← Add this line
	emailService := services.NewEmailService(
		cfg.Email.ResendAPIKey,
		cfg.Email.FromEmail,
		cfg.Email.FromName,
	)
	authHandler := handlers.NewAuthHandler(db, []byte(cfg.JWT.Secret), emailService)
	emailHandler := handlers.NewEmailHandler(emailService, db.DB)
	// Add OTP handler
	otpHandler := handlers.NewOTPHandler(db, emailService, []byte(cfg.JWT.Secret))

	// Health check
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// Public routes
	public := r.Group("/api/v1")
	public.Use(middleware.RateLimiter())
	{
		public.POST("/register", authHandler.Register)
	}

	// Anything that accepts a password or a one-time code is limited per account as
	// well as per IP: 10 attempts a minute is ample for a person mistyping a code, and
	// far too slow to guess a six-digit code or stuff credentials, even from a pool of
	// addresses. The per-code attempt counter backs this up at the database level.
	credentials := r.Group("/api/v1")
	credentials.Use(middleware.RateLimiter(), middleware.CredentialRateLimiter(10, 10))
	{
		credentials.POST("/login", authHandler.Login)
		credentials.POST("/forgot-password", authHandler.ForgotPassword)
		credentials.POST("/reset-password", authHandler.ResetPassword)
	}

	// Protected routes
	protected := r.Group("/api/v1")
	// Authenticated traffic had no ceiling at all, so one client could pin the database
	// pool with report and PDF requests. Limited per account rather than per IP: staff
	// sharing a shop's wifi would otherwise share one budget and throttle each other.
	// 20/sec with a burst of 40 is far above what any screen does — including a fast
	// scroll through a paged list — while still bounding a runaway client.
	protected.Use(
		middleware.AuthMiddleware([]byte(cfg.JWT.Secret), db.DB),
		middleware.UserRateLimiter(20, 40),
	)
	{
		protected.POST("/refresh-token", authHandler.RefreshToken)
		protected.POST("/logout", authHandler.Logout)
		protected.GET("/profile", userHandler.GetUserProfile)

		// Push notification device tokens
		protected.POST("/device-tokens", deviceTokenHandler.Register)
		protected.DELETE("/device-tokens", deviceTokenHandler.Unregister)
		protected.POST("/push/test", deviceTokenHandler.SendTest)

		// Company routes
		protected.POST("/companies", companyHandler.CreateCompany)
		protected.GET("/companies", companyHandler.GetMyCompanies)
		protected.GET("/companies/:companyId/address", companyAddressHandler.GetCompanyAddress)
		protected.POST("/companies/:companyId/address", companyAddressHandler.SaveCompanyAddress)

		// Client routes
		protected.POST("/clients", clientHandler.CreateClient)
		protected.GET("/companies/:companyId/clients", clientHandler.GetClients)
		protected.PUT("/clients/:clientId", clientHandler.UpdateClient)
		protected.DELETE("/clients/:clientId", clientHandler.DeleteClient)
		protected.GET("/clients/:clientId/address", clientAddressHandler.GetClientAddress)
		protected.POST("/clients/:clientId/address", clientAddressHandler.SaveClientAddress)
		// invoices by client
		protected.GET("/clients/:clientId/invoices", invoiceHandler.GetInvoicesByClientID)

		// Item routes
		protected.POST("/items", itemHandler.CreateItem)
		protected.PUT("/items/:itemId", itemHandler.UpdateItem)
		protected.GET("/items/:companyId/all", itemHandler.GetItems)
		protected.GET("/item/:itemId/one", itemHandler.GetItemByID)
		protected.POST("/item/:itemId/restock", itemHandler.RestockItem)
		protected.GET("/item/:itemId/movements", itemHandler.GetItemMovements)

		// Category routes
		protected.POST("/categories", categoryHandler.CreateCategory)
		protected.GET("/categories/:companyId", categoryHandler.GetCategories)

		// Invoice routes
		protected.POST("/invoices", invoiceHandler.CreateInvoice)
		protected.GET("/invoices", invoiceHandler.GetInvoices)
		protected.GET("/invoices/:id", invoiceHandler.GetInvoiceByID)
		protected.GET("/invoices/number-preview", invoiceHandler.GetInvoiceNumberPreview)
		protected.GET("/clients/:clientId/unpaid-invoices", invoiceHandler.GetUnpaidInvoices)
		protected.POST("/invoices/:id/issue", invoiceHandler.IssueInvoice)
		protected.PUT("/invoices/:id/update", invoiceHandler.UpdateInvoice) // 👈 REQUIRED
		protected.DELETE("/invoices/:id", invoiceHandler.DeleteInvoice)
		protected.POST("/invoices/:id/cancel", invoiceHandler.CancelInvoice)

		// Estimate / Quotation routes
		protected.POST("/estimates", estimateHandler.CreateEstimate)
		protected.GET("/estimates", estimateHandler.GetEstimates)
		protected.GET("/estimates/number-preview", estimateHandler.GetEstimateNumberPreview)
		protected.GET("/estimates/:id", estimateHandler.GetEstimateByID)
		protected.PUT("/estimates/:id/update", estimateHandler.UpdateEstimate)
		protected.POST("/estimates/:id/status", estimateHandler.UpdateEstimateStatus)
		protected.POST("/estimates/:id/convert", estimateHandler.ConvertToInvoice)
		protected.GET("/estimates/:id/pdf", estimateHandler.GetEstimatePDF)

		// Expense routes ← Add these lines
		protected.POST("/expenses", expenseHandler.CreateExpense)
		protected.GET("/expenses/:id", expenseHandler.GetExpenseByID)
		protected.PUT("/expenses/:id", expenseHandler.UpdateExpense)
		protected.DELETE("/expenses/:id", expenseHandler.DeleteExpense)
		protected.GET("/companies/:companyId/expenses", expenseHandler.GetExpenses)
		// protected.GET("/companies/:id/expenses/range", expenseHandler.GetExpensesByDateRange)
		// protected.GET("/companies/:id/expenses/stats", expenseHandler.GetExpenseStats)

		protected.GET("/invoices/:id/pdf", invoicePDFHandler.GetInvoicePDF)

		// Ledger routes
		protected.GET("/ledger/:clientId", ledgerHandler.GetClientLedger)
		protected.GET("/companies/:companyId/ledger", ledgerHandler.GetCompanyLedger)

		protected.POST("/payments", paymentHandler.RecordPayment)

		// credit note routes
		protected.POST("/credit-notes", creditNoteHandler.Create)
		protected.GET("/credit-notes", creditNoteHandler.GetAll)
		protected.GET("/credit-notes/:id", creditNoteHandler.GetByID)

		// Dashboard routes
		protected.GET("/dashboard", dashboard.GetDashboard)

		// GST reports
		protected.GET("/companies/:companyId/reports/gstr1", gstReportHandler.GetGSTReport)
		protected.GET("/companies/:companyId/reports/aging", agingReportHandler.GetAgingReport)
		protected.GET("/companies/:companyId/reports/stock", stockReportHandler.GetStockReport)

		protected.GET("/companies/:companyId/banks", companyBankHandlerss.List)
		protected.POST("/companies/:companyId/banks", companyBankHandlerss.Create)
		protected.PUT("/companies/:companyId/banks/:bankId", companyBankHandlerss.Update)

		protected.POST("/invoices/:id/send-email", emailHandler.SendInvoiceEmail)

		// Add to public routes (no auth needed)
		credentials.POST("/send-otp", otpHandler.SendOTP)
		credentials.POST("/verify-otp", otpHandler.VerifyOTP)
		// Add to public routes
		credentials.POST("/verify-email", authHandler.VerifyEmail)
		credentials.POST("/resend-verification", authHandler.ResendVerification)

		protected.DELETE("/account", authHandler.DeleteAccount)
	}
}
