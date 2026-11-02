package handlers_test

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	_ "github.com/lib/pq"
	"golang.org/x/crypto/bcrypt"
	database "invo-server/internal/db"
	"invo-server/internal/handlers"
	"invo-server/internal/middleware"
)

func securityDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("INVO_TEST_DB")
	if dsn == "" {
		t.Skip("set INVO_TEST_DB to a disposable PostgreSQL database")
	}
	admin, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("security_%d", time.Now().UnixNano())
	if _, err = admin.Exec("CREATE SCHEMA " + schema); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	scoped := dsn + " search_path=" + schema
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		u, err := url.Parse(dsn)
		if err != nil {
			t.Fatal(err)
		}
		q := u.Query()
		q.Set("search_path", schema)
		u.RawQuery = q.Encode()
		scoped = u.String()
	}
	db, err := sql.Open("postgres", scoped)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(20)
	t.Cleanup(func() { db.Close(); admin.Exec("DROP SCHEMA " + schema + " CASCADE"); admin.Close() })
	securityExec(t, db, `CREATE TABLE users(id INTEGER PRIMARY KEY,email TEXT,password_hash TEXT,is_verified BOOLEAN DEFAULT TRUE,tokens_valid_from TIMESTAMPTZ DEFAULT NOW(),session_version INT NOT NULL DEFAULT 1);
 CREATE TABLE companies(id INTEGER PRIMARY KEY,user_id INTEGER);
 CREATE TABLE company_members(company_id INTEGER,user_id INTEGER,role TEXT);
 CREATE FUNCTION companies_for_user(uid INTEGER) RETURNS TABLE(company_id INTEGER) LANGUAGE SQL AS $$ SELECT id FROM companies WHERE user_id=uid UNION SELECT company_id FROM company_members WHERE user_id=uid $$;
 CREATE TABLE clients(id INTEGER PRIMARY KEY,company_id INTEGER,name TEXT);
 CREATE TABLE items(id INTEGER PRIMARY KEY,company_id INTEGER,name TEXT);
 CREATE TABLE invoices(id INTEGER PRIMARY KEY,company_id INTEGER,client_id INTEGER);
 CREATE TABLE purchase_returns(id INTEGER PRIMARY KEY,company_id INTEGER);
 CREATE TABLE password_reset_tokens(id SERIAL,email TEXT,code TEXT,attempts INT DEFAULT 0,used BOOLEAN DEFAULT FALSE,expires_at TIMESTAMPTZ,created_at TIMESTAMPTZ DEFAULT NOW());
 CREATE TABLE otp_codes(id SERIAL,email TEXT,code TEXT,attempts INT DEFAULT 0,used BOOLEAN DEFAULT FALSE,expires_at TIMESTAMPTZ,created_at TIMESTAMPTZ DEFAULT NOW());
 INSERT INTO users(id,email,password_hash) VALUES(7,'owner@example.invalid',''),(8,'staff@example.invalid','');
 INSERT INTO companies VALUES(1,7),(2,99);
 INSERT INTO company_members VALUES(1,7,'owner'),(1,8,'staff');
 INSERT INTO clients VALUES(10,1,'Own customer'),(11,1,'Another own customer'),(20,2,'Foreign customer');
 INSERT INTO items VALUES(10,1,'Own item'),(20,2,'Foreign item');
 INSERT INTO invoices VALUES(10,1,10),(11,1,11),(20,2,20);
 INSERT INTO purchase_returns VALUES(10,1),(20,2);`)
	for _, name := range []string{"63_sessions", "68_delivery_challan", "74_session_revocation", "75_challan_integrity"} {
		b, err := os.ReadFile("../../migrations/" + name + ".up.sql")
		if err != nil {
			t.Fatal(err)
		}
		securityExec(t, db, string(b))
	}
	return db
}
func securityExec(t *testing.T, db *sql.DB, q string, args ...any) {
	t.Helper()
	if _, err := db.Exec(q, args...); err != nil {
		t.Fatal(err)
	}
}
func securityRequest(r *gin.Engine, method, path, body, token string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	r.ServeHTTP(w, req)
	return w
}
func securityStatus(t *testing.T, w *httptest.ResponseRecorder, want int) {
	t.Helper()
	if w.Code != want {
		t.Fatalf("want %d got %d: %s", want, w.Code, w.Body.String())
	}
}

type securityTokens struct {
	Token   string `json:"token"`
	Refresh string `json:"refresh_token"`
	ID      string `json:"session_id"`
}

func securityDecode(t *testing.T, w *httptest.ResponseRecorder) securityTokens {
	t.Helper()
	securityStatus(t, w, 200)
	var out securityTokens
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Token == "" || out.Refresh == "" || out.ID == "" {
		t.Fatalf("incomplete credentials: %s", w.Body.String())
	}
	return out
}

func TestSessionRevocationAndRotation(t *testing.T) {
	db := securityDB(t)
	gin.SetMode(gin.TestMode)
	secret := []byte("isolated-test-secret-only")
	ah := handlers.NewAuthHandler(&database.Database{DB: db}, secret, nil)
	sh := handlers.NewSessionHandler(&database.Database{DB: db}, secret, time.Hour)
	oh := handlers.NewOTPHandler(&database.Database{DB: db}, nil, secret)
	r := gin.New()
	r.POST("/refresh", sh.RefreshTokens)
	r.POST("/reset", ah.ResetPassword)
	r.POST("/login", ah.Login)
	r.POST("/verify", ah.VerifyEmail)
	r.POST("/otp", oh.VerifyOTP)
	p := r.Group("")
	p.Use(middleware.AuthMiddleware(secret, db))
	p.GET("/probe", func(c *gin.Context) { c.Status(200) })
	p.POST("/logout", ah.Logout)
	p.POST("/legacy-refresh", ah.RefreshToken)
	p.DELETE("/sessions/:id", sh.RevokeSession)
	p.DELETE("/sessions", sh.RevokeAllOtherSessions)
	add := func(label string) string {
		t.Helper()
		sum := sha256.Sum256([]byte(label))
		var id string
		if err := db.QueryRow(`INSERT INTO sessions(user_id,session_version,refresh_token_hash,expires_at) SELECT id,session_version,$1,NOW()+interval '90 days' FROM users WHERE id=7 RETURNING sessions.id`, hex.EncodeToString(sum[:])).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	refresh := func(token string) *httptest.ResponseRecorder {
		return securityRequest(r, "POST", "/refresh", fmt.Sprintf(`{"refresh_token":%q}`, token), "")
	}
	fresh := func(label string) securityTokens { t.Helper(); add(label); return securityDecode(t, refresh(label)) }
	a := fresh("one")
	b := fresh("two")
	securityStatus(t, securityRequest(r, "DELETE", "/sessions/"+b.ID, "", a.Token), 200)
	securityStatus(t, securityRequest(r, "GET", "/probe", "", b.Token), 401)
	securityStatus(t, refresh(b.Refresh), 401)
	securityStatus(t, securityRequest(r, "GET", "/probe", "", a.Token), 200)
	securityStatus(t, securityRequest(r, "DELETE", "/sessions?keep="+b.ID, "", a.Token), 400)
	other := fresh("other")
	securityStatus(t, securityRequest(r, "DELETE", "/sessions?keep="+a.ID, "", a.Token), 200)
	securityStatus(t, securityRequest(r, "GET", "/probe", "", other.Token), 401)
	securityStatus(t, securityRequest(r, "GET", "/probe", "", a.Token), 200)
	securityStatus(t, securityRequest(r, "POST", "/logout", "", a.Token), 200)
	securityStatus(t, refresh(a.Refresh), 401)
	securityStatus(t, securityRequest(r, "GET", "/probe", "", a.Token), 401)

	beforeReset := fresh("before-reset")
	hash, _ := bcrypt.GenerateFromPassword([]byte("123456"), bcrypt.MinCost)
	securityExec(t, db, `INSERT INTO password_reset_tokens(email,code,expires_at) VALUES('owner@example.invalid',$1,NOW()+interval '1 hour')`, string(hash))
	reset := securityDecode(t, securityRequest(r, "POST", "/reset", `{"email":"owner@example.invalid","code":"123456","new_password":"new-secure-password"}`, ""))
	securityStatus(t, refresh(beforeReset.Refresh), 401)
	securityStatus(t, securityRequest(r, "GET", "/probe", "", beforeReset.Token), 401)
	securityStatus(t, securityRequest(r, "GET", "/probe", "", reset.Token), 200)
	// One refresh credential can be spent only once, even when requests start together.
	race := fresh("race")
	start := make(chan struct{})
	results := make(chan *httptest.ResponseRecorder, 12)
	for i := 0; i < 12; i++ {
		go func() { <-start; results <- refresh(race.Refresh) }()
	}
	close(start)
	success := 0
	for i := 0; i < 12; i++ {
		w := <-results
		if w.Code == 200 {
			success++
			securityDecode(t, w)
		} else {
			securityStatus(t, w, 401)
		}
	}
	if success != 1 {
		t.Fatalf("concurrent refresh successes = %d", success)
	}
	// Every successful authentication path creates a device-bound access/refresh pair.
	login := securityDecode(t, securityRequest(r, "POST", "/login", `{"email":"owner@example.invalid","password":"new-secure-password","platform":"ios"}`, ""))
	securityStatus(t, securityRequest(r, "GET", "/probe", "", login.Token), 200)
	for _, path := range []string{"/verify", "/otp"} {
		securityExec(t, db, `INSERT INTO otp_codes(email,code,expires_at) VALUES('owner@example.invalid',$1,NOW()+interval '1 hour')`, string(hash))
		out := securityDecode(t, securityRequest(r, "POST", path, `{"email":"owner@example.invalid","code":"123456"}`, ""))
		securityStatus(t, securityRequest(r, "GET", "/probe", "", out.Token), 200)
	}
	// Logout wins against any in-flight refresh: no returned access/refresh pair survives.
	for i := 0; i < 3; i++ {
		old := fresh(fmt.Sprintf("logout-race-%d", i))
		start := make(chan struct{})
		var wg sync.WaitGroup
		var refreshed, loggedOut *httptest.ResponseRecorder
		wg.Add(2)
		go func() { defer wg.Done(); <-start; refreshed = refresh(old.Refresh) }()
		go func() { defer wg.Done(); <-start; loggedOut = securityRequest(r, "POST", "/logout", "", old.Token) }()
		close(start)
		wg.Wait()
		securityStatus(t, loggedOut, 200)
		if refreshed.Code == 200 {
			out := securityDecode(t, refreshed)
			securityStatus(t, refresh(out.Refresh), 401)
			securityStatus(t, securityRequest(r, "GET", "/probe", "", out.Token), 401)
		} else {
			securityStatus(t, refreshed, 401)
		}
	}
	// Expired/unbound legacy JWTs must not regain a session after explicit revocation.
	var version int
	db.QueryRow(`SELECT session_version FROM users WHERE id=7`).Scan(&version)
	legacy, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"user_id": 7, "email": "owner@example.invalid", "sv": version, "iat": time.Now().Unix(), "exp": time.Now().Add(time.Hour).Unix()}).SignedString(secret)
	securityStatus(t, securityRequest(r, "GET", "/probe", "", legacy), 401)
}

func TestChallanTenantValidationPagingAndPolicy(t *testing.T) {
	db := securityDB(t)
	gin.SetMode(gin.TestMode)
	h := handlers.NewChallanHandler(&database.Database{DB: db})
	router := func(user int) *gin.Engine {
		r := gin.New()
		r.Use(func(c *gin.Context) { c.Set("user_id", user); c.Next() }, middleware.Permissions(db))
		r.POST("/api/v1/challans", h.CreateChallan)
		r.GET("/api/v1/challans", h.ListChallans)
		r.GET("/api/v1/challans/:id", h.GetChallanByID)
		r.PATCH("/api/v1/challans/:id/dispatch", h.DispatchChallan)
		r.GET("/api/v1/companies/:companyId/reports/supplier-aging", func(c *gin.Context) { c.Status(200) })
		r.GET("/api/v1/purchase-returns/:id/pdf", func(c *gin.Context) { c.Status(200) })
		return r
	}
	r := router(7)
	for _, body := range []string{
		`{"company_id":1,"client_id":20,"challan_date":"2026-10-12","items":[{"item_id":10,"qty":1}]}`,
		`{"company_id":1,"client_id":10,"invoice_id":20,"challan_date":"2026-10-12","items":[{"item_id":10,"qty":1}]}`,
		`{"company_id":1,"client_id":10,"invoice_id":11,"challan_date":"2026-10-12","items":[{"item_id":10,"qty":1}]}`,
		`{"company_id":1,"client_id":10,"challan_date":"2026-10-12","items":[{"item_id":10,"qty":1},{"item_id":20,"qty":1}]}`,
		`{"company_id":1,"client_id":10,"challan_date":"2026-02-30","items":[{"item_id":10,"qty":1}]}`,
		`{"company_id":1,"client_id":10,"challan_date":"2026-10-12","items":[{"item_id":10,"qty":0}]}`,
		`{"company_id":1,"client_id":10,"challan_date":"2026-10-12","items":[]}`,
	} {
		securityStatus(t, securityRequest(r, "POST", "/api/v1/challans", body, ""), 400)
	}
	var count int
	db.QueryRow(`SELECT COUNT(*) FROM delivery_challans`).Scan(&count)
	if count != 0 {
		t.Fatalf("invalid requests wrote %d challans", count)
	}
	var id int
	for i := 0; i < 3; i++ {
		w := securityRequest(r, "POST", "/api/v1/challans", `{"company_id":1,"client_id":10,"invoice_id":10,"challan_date":"2026-10-12","items":[{"item_id":10,"qty":2}]}`, "")
		securityStatus(t, w, 201)
		var out struct {
			ID int `json:"id"`
		}
		json.Unmarshal(w.Body.Bytes(), &out)
		id = out.ID
	}
	securityStatus(t, securityRequest(r, "GET", fmt.Sprintf("/api/v1/challans/%d", id), "", ""), 200)
	securityStatus(t, securityRequest(r, "PATCH", fmt.Sprintf("/api/v1/challans/%d/dispatch", id), "", ""), 200)
	securityStatus(t, securityRequest(r, "PATCH", fmt.Sprintf("/api/v1/challans/%d/dispatch", id), "", ""), 409)
	for _, tc := range []struct {
		query        string
		count, total int
		more         bool
	}{{"limit=2&offset=0", 2, 3, true}, {"limit=2&offset=2", 1, 3, false}, {"search=missing&limit=2", 0, 0, false}} {
		w := securityRequest(r, "GET", "/api/v1/challans?company_id=1&"+tc.query, "", "")
		securityStatus(t, w, 200)
		var out struct {
			Challans []any `json:"challans"`
			Total    int   `json:"total"`
			More     bool  `json:"has_more"`
		}
		json.Unmarshal(w.Body.Bytes(), &out)
		if len(out.Challans) != tc.count || out.Total != tc.total || out.More != tc.more {
			t.Fatal(w.Body.String())
		}
	}
	if _, err := db.Exec(`INSERT INTO delivery_challan_items(challan_id,company_id,item_id,qty) VALUES($1,1,20,1)`, id); err == nil {
		t.Fatal("database allowed a foreign-company item")
	}
	for _, path := range []string{"/api/v1/companies/1/reports/supplier-aging", "/api/v1/purchase-returns/10/pdf"} {
		securityStatus(t, securityRequest(router(7), "GET", path, "", ""), 200)
		securityStatus(t, securityRequest(router(8), "GET", path, "", ""), 403)
		securityStatus(t, securityRequest(router(99), "GET", path, "", ""), 403)
	}
	securityStatus(t, securityRequest(router(7), "GET", "/api/v1/purchase-returns/20/pdf?company_id=1", "", ""), 403)
}
