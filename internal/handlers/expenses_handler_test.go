package handlers

import (
	"database/sql"
	"fmt"
	database "invo-server/internal/db"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestExpensePaymentMethod(t *testing.T) {
	dsn := os.Getenv("INVO_TEST_DB")
	if dsn == "" {
		t.Skip("set INVO_TEST_DB to run the expense regression")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	schema := fmt.Sprintf("expense_test_%d", time.Now().UnixNano())
	exec := func(query string) {
		t.Helper()
		if _, err := db.Exec(query); err != nil {
			t.Fatal(err)
		}
	}
	exec("CREATE SCHEMA " + schema)
	defer func() { _, _ = db.Exec("DROP SCHEMA " + schema + " CASCADE") }()
	exec("SET search_path TO " + schema)
	exec(`CREATE TABLE companies (id BIGINT, user_id BIGINT);
	 CREATE TABLE company_members (company_id BIGINT, user_id BIGINT);
	 CREATE TABLE expensess (id BIGSERIAL PRIMARY KEY, company_id BIGINT,
	 user_id BIGINT, name TEXT, amount NUMERIC, description TEXT, date DATE,
	 payment_method TEXT, updated_at TIMESTAMP);
	 INSERT INTO companies VALUES (1, 1);`)
	handler := NewExpenseHandler(&database.Database{DB: db})
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("user_id", 1); c.Next() })
	router.POST("/expenses", handler.CreateExpense)
	router.PUT("/expenses/:id", handler.UpdateExpense)
	request := func(method, path, body string, status int) {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		if response.Code != status {
			t.Fatalf("status %d: %s", response.Code, response.Body.String())
		}
	}
	request("POST", "/expenses", `{"company_id":1,"name":"Delivery","amount":25,"date":"2026-10-02","payment_method":"Cash"}`, 201)
	check := func(want string, valid bool) {
		t.Helper()
		var method sql.NullString
		var date string
		if err := db.QueryRow("SELECT payment_method, date::text FROM expensess WHERE id=1").Scan(&method, &date); err != nil {
			t.Fatal(err)
		}
		if method.String != want || method.Valid != valid || date != "2026-10-02" {
			t.Fatalf("method = %+v, date = %s", method, date)
		}
	}
	check("Cash", true)
	base := `{"name":"Delivery","amount":25,"date":"2026-10-02"`
	// Existing clients omit the new field. Their edits must keep its saved value.
	request("PUT", "/expenses/1", base+`}`, 200)
	check("Cash", true)
	request("PUT", "/expenses/1", base+`,"payment_method":"UPI"}`, 200)
	check("UPI", true)
	request("PUT", "/expenses/1", base+`,"payment_method":null}`, 200)
	check("UPI", true)
	request("PUT", "/expenses/1", base+`,"payment_method":""}`, 200)
	check("", false)
	request("PUT", "/expenses/1", base+`,"payment_method":"Cash"}`, 200)
	check("Cash", true)
}
