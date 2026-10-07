package handlers_test

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	_ "github.com/lib/pq"
	database "invo-server/internal/db"
	"invo-server/internal/handlers"
	"invo-server/internal/middleware"
	"invo-server/internal/services"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestReviewSecurityRegressions(t *testing.T) {
	if os.Getenv("INVO_TEST_DB") == "" {
		t.Skip("set INVO_TEST_DB to use an isolated test schema")
	}
	db, err := sql.Open("postgres", os.Getenv("INVO_TEST_DB"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	schema := fmt.Sprintf("review_scope_%d", time.Now().UnixNano())
	exec := func(q string) {
		t.Helper()
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	exec("CREATE SCHEMA " + schema)
	defer func() { _, _ = db.Exec("DROP SCHEMA " + schema + " CASCADE") }()
	exec("SET search_path TO " + schema)
	exec(`CREATE TABLE companies (id INTEGER PRIMARY KEY, user_id INTEGER);
 CREATE TABLE company_members (id INTEGER, company_id INTEGER, user_id INTEGER, role TEXT);
 CREATE TABLE categories (id INTEGER PRIMARY KEY, company_id INTEGER);
 CREATE FUNCTION companies_for_user(uid INTEGER) RETURNS TABLE(company_id INTEGER) LANGUAGE SQL AS $$
 SELECT id FROM companies WHERE user_id = uid UNION SELECT company_id FROM company_members WHERE user_id = uid $$;
 CREATE TABLE items (id SERIAL, name TEXT, category_id INTEGER, sku TEXT, unit TEXT, description TEXT,
 cost_price NUMERIC, price NUMERIC, quantity INTEGER, low_stock_alert INTEGER, tax_rate NUMERIC,
 hsn_code TEXT, company_id INTEGER, user_id INTEGER);
 INSERT INTO companies VALUES (1,7),(2,8);
 INSERT INTO company_members VALUES (1,1,7,'owner'),(2,2,7,'staff');
 INSERT INTO categories VALUES (20,2),(10,1);`)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("user_id", 7); c.Next() }, middleware.Permissions(db))
	r.POST("/api/v1/items", handlers.NewItemHandler(&database.Database{DB: db}).CreateItem)
	request := func(path, body string) int {
		req := httptest.NewRequest("POST", path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		t.Logf("%s => %d %s", path, w.Code, w.Body.String())
		return w.Code
	}
	body := `{"company_id":2,"category_id":20,"name":"Review item","price":100,"quantity":1}`
	if got := request("/api/v1/items", body); got != 403 {
		t.Fatalf("baseline %d", got)
	}
	if got := request("/api/v1/items?company_id=1", body); got != 403 {
		t.Fatalf("cross-company create should be denied: %d", got)
	}
	t.Log("FIXED: conflicting query/body item company is refused")
	if got := request("/api/v1/items", `{"company_id":1,"name":"No category","price":100,"quantity":1}`); got != 201 {
		t.Fatalf("uncategorized item %d", got)
	}
	t.Log("FIXED: owner can create uncategorized item")
	// The import endpoints use the same body resolver, with file content over 1 MiB.
	r.POST("/api/v1/items/import/preview", func(c *gin.Context) { c.Status(200) })
	small, _ := json.Marshal(map[string]any{"company_id": 1, "csv": "Name,Price\nExample,10"})
	large, _ := json.Marshal(map[string]any{"company_id": 1, "csv": strings.Repeat("A", (1<<20)+10)})
	if got := request("/api/v1/items/import/preview", string(small)); got != 200 {
		t.Fatalf("small import %d", got)
	}
	if got := request("/api/v1/items/import/preview", string(large)); got != 200 {
		t.Fatalf("large import %d", got)
	}
	t.Log("FIXED: authorized import body >1 MiB reaches handler")
	exec(`CREATE TABLE expensess (id INTEGER, name TEXT, amount NUMERIC, description TEXT,
 date DATE, company_id INTEGER, user_id INTEGER, payment_method TEXT, created_at TIMESTAMP, updated_at TIMESTAMP);
 INSERT INTO expensess VALUES (1,'Review expense',100,'fixture','2026-10-07',2,8,'Cash',NOW(),NOW());`)
	expenseHandler := handlers.NewExpenseHandler(&database.Database{DB: db})
	r.GET("/api/v1/companies/:companyId/expenses", expenseHandler.GetExpenses)
	r.GET("/api/v1/expenses/:id", expenseHandler.GetExpenseByID)
	r.PUT("/api/v1/expenses/:id", expenseHandler.UpdateExpense)
	call := func(method, path, body string, want int) {
		t.Helper()
		w := httptest.NewRecorder()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		t.Logf("%s %s => %d", method, path, w.Code)
		if w.Code != want {
			t.Fatalf("expected %d, got %d %s", want, w.Code, w.Body.String())
		}
	}
	call("GET", "/api/v1/companies/2/expenses", "", 403)
	call("GET", "/api/v1/expenses/1", "", 403)
	call("PUT", "/api/v1/expenses/1", `{"name":"Changed by staff","amount":500,"date":"2026-10-07"}`, 403)
	t.Log("FIXED: staff denied individual expense reads and changes")
	t.Setenv("REQUIRE_REAUTH", "false")
	exec(`CREATE TABLE company_bank_accounts (id INTEGER PRIMARY KEY,company_id INTEGER,is_default BOOLEAN,updated_at TIMESTAMP);
 INSERT INTO company_bank_accounts VALUES (201,2,false,NOW());`)
	r.DELETE("/api/v1/companies/:companyId/banks/:bankId", handlers.NewCompanyBankHandler(db).Delete)
	call("DELETE", "/api/v1/companies/2/banks/201", "", 403)
	call("DELETE", "/api/v1/companies/1/banks/201", "", 403)
	var banks int
	if err := db.QueryRow("SELECT COUNT(*) FROM company_bank_accounts WHERE id=201").Scan(&banks); err != nil {
		t.Fatal(err)
	}
	if banks != 1 {
		t.Fatal("staff company bank was deleted")
	}
	exec("INSERT INTO company_bank_accounts VALUES (101,1,false,NOW())")
	call("DELETE", "/api/v1/companies/1/banks/101", "", 204)
	t.Log("Bank deletion checks its stored company and returns 204 on success")

	// Exercise the purchase routes against their migrations, not a guessed schema.
	exec(`ALTER TABLE items ADD PRIMARY KEY (id);
 ALTER TABLE companies ADD COLUMN name TEXT, ADD COLUMN gst TEXT, ADD COLUMN phone TEXT,
 ADD COLUMN address TEXT, ADD COLUMN city TEXT, ADD COLUMN state TEXT, ADD COLUMN pincode TEXT;
 CREATE TABLE stock_movements (item_id BIGINT, company_id BIGINT, user_id BIGINT,
 movement_type TEXT, quantity_change INTEGER, previous_quantity INTEGER, new_quantity INTEGER, reference TEXT, note TEXT);`)
	for _, path := range []string{"../../migrations/55_purchases.up.sql", "../../migrations/62_purchase_returns.up.sql"} {
		migration, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		exec(string(migration))
	}
	exec(`INSERT INTO suppliers (id,company_id,user_id,name,address,pincode,notes) VALUES
 (1,1,7,'Supplier A','Saved address','560001','Saved note'),(2,2,8,'Other supplier','','','');
 INSERT INTO purchase_bills (company_id,user_id,supplier_id,bill_number,total,remaining_amount)
 SELECT 1,7,1,'BILL-' || n,100,100 FROM generate_series(1,61) n;`)
	purchase := handlers.NewPurchaseHandler(db, services.NewPurchaseService(db))
	r.GET("/api/v1/purchase-bills", purchase.GetBills)
	r.GET("/api/v1/suppliers", purchase.GetSuppliers)
	r.PUT("/api/v1/suppliers/:id", purchase.UpdateSupplier)
	r.POST("/api/v1/purchase-bills/:id/cancel", purchase.CancelBill)
	r.GET("/api/v1/suppliers/:id/ledger/statement.pdf", purchase.GetSupplierStatementPDF)
	call("PUT", "/api/v1/suppliers/2?company_id=1", `{"name":"Attack"}`, 403)
	call("PUT", "/api/v1/suppliers/1?company_id=1", `{"name":"Updated supplier"}`, 204)
	var address, pincode, notes string
	if err := db.QueryRow(`SELECT address,pincode,notes FROM suppliers WHERE id=1`).Scan(&address, &pincode, &notes); err != nil {
		t.Fatal(err)
	}
	if address != "Saved address" || pincode != "560001" || notes != "Saved note" {
		t.Fatal("edit cleared omitted supplier details")
	}
	fetch := func(path string) *httptest.ResponseRecorder {
		t.Helper()
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 200 {
			t.Fatalf("GET %s: %d %s", path, w.Code, w.Body.String())
		}
		return w
	}
	for _, tc := range []struct {
		query string
		count int
	}{{"", 50}, {"&offset=50", 11}, {"&search=BILL-1&limit=100", 11}, {"&search=no-match", 0}} {
		response := fetch("/api/v1/purchase-bills?company_id=1" + tc.query)
		var page struct {
			Data []json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &page); err != nil {
			t.Fatal(err)
		}
		if len(page.Data) != tc.count {
			t.Fatalf("bill page %s: got %d want %d", tc.query, len(page.Data), tc.count)
		}
	}
	suppliers := fetch("/api/v1/suppliers?company_id=1")
	if !strings.Contains(suppliers.Body.String(), "Saved address") || !strings.Contains(suppliers.Body.String(), "Saved note") {
		t.Fatal("supplier details not returned for edit")
	}
	statement := fetch("/api/v1/suppliers/1/ledger/statement.pdf?company_id=1&start=2026-01-01&end=2026-12-31")
	if !strings.HasPrefix(statement.Body.String(), "%PDF-") {
		t.Fatal("statement endpoint didn't produce a PDF")
	}
	call("POST", "/api/v1/purchase-bills/1/cancel?company_id=1", "", 204)
	call("POST", "/api/v1/purchase-bills/1/cancel?company_id=1", "", 400)
	t.Log("Supplier edit preserves details; 61 bills page/search correctly; supplier PDF and cancellation endpoints work")
	// A real blocked insert must respect the request deadline and leave no row behind.
	lockDB, err := sql.Open("postgres", os.Getenv("INVO_TEST_DB"))
	if err != nil {
		t.Fatal(err)
	}
	defer lockDB.Close()
	tx, err := lockDB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err = tx.Exec("LOCK TABLE " + schema + ".items IN ACCESS EXCLUSIVE MODE"); err != nil {
		t.Fatal(err)
	}
	slow := gin.New()
	slow.Use(func(c *gin.Context) { c.Set("user_id", 7); c.Next() }, middleware.RequestDeadline(100*time.Millisecond), middleware.Permissions(db))
	slow.POST("/api/v1/items", handlers.NewItemHandler(&database.Database{DB: db}).CreateItem)
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/items", strings.NewReader(`{"company_id":1,"name":"Blocked insert","price":100,"quantity":1}`))
	req.Header.Set("Content-Type", "application/json")
	start := time.Now()
	done := make(chan struct{})
	go func() { slow.ServeHTTP(w, req); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		tx.Rollback()
		<-done
		t.Fatal("handler ignored the request deadline")
	}
	if w.Code < 400 {
		t.Fatalf("blocked insert succeeded: %d", w.Code)
	}
	t.Logf("Blocked endpoint stopped after %s", time.Since(start))
	tx.Rollback()
	// lib/pq discards a cancelled connection, so qualify the schema on cleanup/check.
	var inserted int
	if err := db.QueryRow("SELECT COUNT(*) FROM " + schema + ".items WHERE name='Blocked insert'").Scan(&inserted); err != nil {
		t.Fatal(err)
	}
	if inserted != 0 {
		t.Fatal("cancelled insert was saved")
	}

}
