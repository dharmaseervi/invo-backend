package routes

import (
	"log"
	"sort"
	"strings"
	"time"

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
	profitLossHandler := handlers.NewProfitLossHandler(db.DB)
	estimateHandler := handlers.NewEstimateHandler(db)
	pushService := services.NewPushService(db.DB, cfg)
	deviceTokenHandler := handlers.NewDeviceTokenHandler(pushService)
	services.StartOverdueChecker(db.DB, pushService, cfg.OverdueCheckInterval)

	ledgerService := services.NewLedgerService(db.DB)
	ledgerHandler := handlers.NewLedgerHandler(ledgerService, db.DB)
	invoiceHandler := handlers.NewInvoiceHandler(db, ledgerService, pushService)
	creditNoteService := services.NewCreditNoteService(db.DB, ledgerService)
	purchaseService := services.NewPurchaseService(db.DB)
	purchaseHandler := handlers.NewPurchaseHandler(db.DB, purchaseService)
	staffHandler := handlers.NewStaffHandler(db.DB)
	stocktakeService := services.NewStocktakeService(db.DB)
	closingHandler := handlers.NewClosingHandler(db.DB, stocktakeService, ledgerService)

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
		credentials.POST("/forgot-password", middleware.EmailQuota(8, 4, 40, 15), authHandler.ForgotPassword)
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
		// Every request gets a deadline. A slow query used to hold its database
		// connection until it finished however long that took, with the caller long
		// gone — and it takes few of those to stop the shop billing anybody at all.
		// 30s is far above what any screen needs and still bounds a runaway.
		middleware.RequestDeadline(30*time.Second),
		middleware.AuthMiddleware([]byte(cfg.JWT.Secret), db.DB),
		middleware.UserRateLimiter(20, 40),
		// What this account may do in the business the request is about. Runs for
		// every protected route and governs the ones named in the policy; the rest
		// pass through to the handler's own membership check, as before.
		middleware.Permissions(db.DB),
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
		protected.PUT("/companies/:companyId", companyHandler.UpdateCompany)
		protected.GET("/companies/:companyId/address", companyAddressHandler.GetCompanyAddress)
		protected.POST("/companies/:companyId/address", companyAddressHandler.SaveCompanyAddress)

		// Client routes
		protected.POST("/clients", clientHandler.CreateClient)
		protected.GET("/companies/:companyId/clients", clientHandler.GetClients)
		// The Cash/UPI account a walk-in sale is billed to, found by name rather than
		// by the app searching a list it would otherwise have to hold all of.
		protected.POST("/companies/:companyId/quick-sale-client", clientHandler.QuickSaleClient)
		protected.PUT("/clients/:clientId", clientHandler.UpdateClient)
		protected.DELETE("/clients/:clientId", clientHandler.DeleteClient)
		protected.GET("/clients/:clientId/address", clientAddressHandler.GetClientAddress)
		protected.POST("/clients/:clientId/address", clientAddressHandler.SaveClientAddress)
		// invoices by client
		protected.GET("/clients/:clientId/invoices", invoiceHandler.GetInvoicesByClientID)

		// Item routes
		protected.POST("/items", itemHandler.CreateItem)
		// Catalogue import: preview first, which writes nothing, then apply the rows
		// the person kept.
		protected.POST("/items/import/preview", itemHandler.PreviewItemImport)
		protected.POST("/items/import", itemHandler.ApplyItemImport)
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
		// Totals and counts over every matching invoice, not just the page on screen.
		protected.GET("/invoices/summary", invoiceHandler.GetInvoiceSummary)
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
		// This month, last month and the total, over every expense — the rows are paged.
		protected.GET("/companies/:companyId/expenses/summary", expenseHandler.GetExpenseSummary)
		// protected.GET("/companies/:id/expenses/range", expenseHandler.GetExpensesByDateRange)
		// protected.GET("/companies/:id/expenses/stats", expenseHandler.GetExpenseStats)

		protected.GET("/invoices/:id/pdf", invoicePDFHandler.GetInvoicePDF)

		// Ledger routes
		protected.GET("/ledger/:clientId", ledgerHandler.GetClientLedger)
		// Totals over a customer's whole history, so the rows can be paged without the
		// figures becoming the figures of a page.
		protected.GET("/ledger/:clientId/summary", ledgerHandler.GetClientLedgerSummary)
		// The statement a shop sends its customer, as a PDF.
		protected.GET("/ledger/:clientId/statement.pdf", ledgerHandler.GetClientStatementPDF)
		protected.GET("/companies/:companyId/ledger", ledgerHandler.GetCompanyLedger)
		// One row per customer with history: what a ledger list screen actually shows.
		protected.GET("/companies/:companyId/ledger/summary", ledgerHandler.GetCompanyLedgerSummaries)

		protected.POST("/payments", paymentHandler.RecordPayment)
		// Putting a payment right: undo one recorded by mistake, or move it onto the
		// invoices it should have settled.
		protected.POST("/payments/:id/reverse", paymentHandler.ReversePayment)
		protected.PUT("/payments/:id/allocations", paymentHandler.ReallocatePayment)
		// Money going back to a customer.
		protected.POST("/refunds", paymentHandler.RecordRefund)
		protected.GET("/refunds", paymentHandler.GetRefunds)
		protected.GET("/companies/:companyId/payments", paymentHandler.GetPayments)

		// Purchases: suppliers, their bills, and what the shop owes them.
		protected.POST("/suppliers", purchaseHandler.CreateSupplier)
		protected.GET("/suppliers", purchaseHandler.GetSuppliers)
		protected.PUT("/suppliers/:id", purchaseHandler.UpdateSupplier)
		protected.POST("/purchase-bills", purchaseHandler.RecordBill)
		protected.GET("/purchase-bills", purchaseHandler.GetBills)
		protected.GET("/purchase-bills/:id", purchaseHandler.GetBill)
		protected.POST("/purchase-bills/:id/cancel", purchaseHandler.CancelBill)
		protected.POST("/supplier-payments", purchaseHandler.PaySupplier)
		// Stock going back to a supplier.
		protected.POST("/purchase-returns", purchaseHandler.RecordReturn)
		protected.GET("/purchase-returns", purchaseHandler.GetReturns)
		protected.GET("/suppliers/:id/ledger", purchaseHandler.SupplierLedger)
		protected.GET("/suppliers/:id/ledger/summary", purchaseHandler.SupplierLedgerSummary)
		protected.GET("/suppliers/:id/ledger/statement.pdf", purchaseHandler.GetSupplierStatementPDF)

		// Closing the day: counting the floor, and counting the drawer.
		protected.POST("/stocktakes", closingHandler.StartStocktake)
		protected.GET("/stocktakes/:id", closingHandler.GetStocktake)
		protected.POST("/stocktakes/:id/count", closingHandler.CountItem)
		protected.POST("/stocktakes/:id/apply", closingHandler.ApplyStocktake)
		protected.DELETE("/stocktakes/:id", closingHandler.AbandonStocktake)
		protected.GET("/day-closing", closingHandler.GetDayClosing)
		protected.POST("/day-closing", closingHandler.CloseDay)
		protected.GET("/day-closings", closingHandler.GetRecentClosings)

		// Who works here. Owner only — enforced by the permission policy, not here.
		protected.GET("/companies/:companyId/staff", staffHandler.GetStaff)
		protected.POST("/companies/:companyId/staff", staffHandler.AddStaff)
		protected.PUT("/companies/:companyId/staff/:memberId", staffHandler.UpdateStaff)
		protected.DELETE("/companies/:companyId/staff/:memberId", staffHandler.RemoveStaff)

		// credit note routes
		protected.POST("/credit-notes", creditNoteHandler.Create)
		protected.GET("/credit-notes", creditNoteHandler.GetAll)
		// Before the :id route, or "summary" is read as a credit note id.
		protected.GET("/credit-notes/summary", creditNoteHandler.GetSummary)
		protected.GET("/credit-notes/:id", creditNoteHandler.GetByID)
		// Deciding which invoice a credit note settles.
		protected.POST("/credit-notes/:id/apply", creditNoteHandler.ApplyToInvoice)

		// Dashboard routes
		protected.GET("/dashboard", dashboard.GetDashboard)

		// GST reports
		protected.GET("/companies/:companyId/reports/gstr1", gstReportHandler.GetGSTReport)
		protected.GET("/companies/:companyId/reports/aging", agingReportHandler.GetAgingReport)
		protected.GET("/companies/:companyId/reports/stock", stockReportHandler.GetStockReport)
		protected.GET("/companies/:companyId/reports/profit-loss", profitLossHandler.GetProfitLoss)

		protected.GET("/companies/:companyId/banks", companyBankHandlerss.List)
		protected.POST("/companies/:companyId/banks", companyBankHandlerss.Create)
		protected.PUT("/companies/:companyId/banks/:bankId", companyBankHandlerss.Update)
		protected.DELETE("/companies/:companyId/banks/:bankId", companyBankHandlerss.Delete)

		protected.POST("/invoices/:id/send-email", emailHandler.SendInvoiceEmail)

		// Add to public routes (no auth needed)
		// Endpoints that send a real email are additionally capped per address and per
		// source: 8 an hour is more than anyone needs, and far less than a flood.
		mailQuota := middleware.EmailQuota(8, 4, 40, 15)
		credentials.POST("/send-otp", mailQuota, otpHandler.SendOTP)
		credentials.POST("/verify-otp", otpHandler.VerifyOTP)
		// Add to public routes
		credentials.POST("/verify-email", authHandler.VerifyEmail)
		credentials.POST("/resend-verification", mailQuota, authHandler.ResendVerification)

		protected.DELETE("/account", authHandler.DeleteAccount)
	}

	assertPermissionPolicyMatchesRoutes(r)
}

// assertPermissionPolicyMatchesRoutes refuses to start if the permission policy names a
// route that no longer exists.
//
// The policy is a table of route patterns, which is what makes it readable in one place
// — and what lets it rot in silence. Rename a route and its entry stops matching: the
// route keeps working, for everybody, with the permission it was supposed to need
// quietly gone. Nothing fails, no test goes red, and the hole is found the day a counter
// boy opens the day's takings.
//
// So it is checked against the router that was just built, at startup, where it is loud.
func assertPermissionPolicyMatchesRoutes(r *gin.Engine) {
	registered := make(map[string]bool)
	for _, route := range r.Routes() {
		registered[route.Method+" "+route.Path] = true
	}

	var missing []string
	for _, governed := range middleware.GovernedRoutes() {
		if !registered[governed] {
			missing = append(missing, governed)
		}
	}

	if len(missing) > 0 {
		sort.Strings(missing)
		log.Fatalf(
			"permission policy names %d route(s) that do not exist: %s",
			len(missing), strings.Join(missing, ", "),
		)
	}
}
