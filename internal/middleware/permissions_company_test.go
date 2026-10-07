package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// resolveVia runs companyForRequest the way the middleware does, with no database, so
// these cases must use routes that name no record. The record lookup is the one branch
// that needs Postgres, and it is covered by the live permission tests.
func resolveVia(t *testing.T, method, pattern, target, body string) (int64, bool) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	var (
		got   int64
		ok    bool
		ran   bool
		route = gin.New()
	)

	handler := func(c *gin.Context) {
		ran = true
		got, ok = companyForRequest(c, nil)
		c.Status(http.StatusOK)
	}
	route.Handle(method, pattern, handler)

	var reader *strings.Reader
	if body == "" {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, target, reader)
	req.Header.Set("Content-Type", "application/json")
	route.ServeHTTP(httptest.NewRecorder(), req)

	if !ran {
		t.Fatalf("the route %s %s never matched %s", method, pattern, target)
	}
	return got, ok
}

// The escalation this closes: a user who is an owner of one company and staff in
// another said the company they own in the query string and the company they are only
// staff in in the body. The permission check asked about the first and the handler
// acted on the second, so a role that had been refused the catalogue could write to it.
func TestCompanyForRequestRefusesDisagreeingIdentifiers(t *testing.T) {
	if _, ok := resolveVia(t, http.MethodPost, "/api/v1/items", "/api/v1/items?company_id=7",
		`{"company_id":9,"name":"Tile"}`); ok {
		t.Error("a request naming company 7 in the query and 9 in the body was resolved; it must be refused")
	}

	// The camel-cased spelling is the same request wearing a different hat.
	if _, ok := resolveVia(t, http.MethodPost, "/api/v1/items", "/api/v1/items?companyId=7",
		`{"companyId":9,"name":"Tile"}`); ok {
		t.Error("the camelCase spelling of the same disagreement was resolved")
	}

	// And one of each spelling, which is how it would be smuggled past a resolver that
	// compared only like with like.
	if _, ok := resolveVia(t, http.MethodPost, "/api/v1/items", "/api/v1/items?company_id=7",
		`{"companyId":9,"name":"Tile"}`); ok {
		t.Error("a snake_case query against a camelCase body was resolved")
	}
}

// Refusing disagreement is only safe if agreement still works. The apps send the
// company in whichever place each endpoint settled on years ago, and sometimes in two
// places at once; every one of those is a legitimate request.
func TestCompanyForRequestAcceptsAgreement(t *testing.T) {
	cases := []struct {
		name, method, pattern, target, body string
	}{
		{"query only", http.MethodGet, "/api/v1/dashboard", "/api/v1/dashboard?companyId=7", ""},
		{"body only, as the catalogue import sends it", http.MethodPost,
			"/api/v1/items/import", "/api/v1/items/import", `{"company_id":7,"csv":"Name\nTile\n"}`},
		{"path param", http.MethodGet, "/api/v1/companies/:companyId/expenses",
			"/api/v1/companies/7/expenses", ""},
		{"query and body saying the same thing", http.MethodPost, "/api/v1/items",
			"/api/v1/items?company_id=7", `{"company_id":7,"name":"Tile"}`},
		{"path and body saying the same thing", http.MethodPost,
			"/api/v1/companies/:companyId/banks", "/api/v1/companies/7/banks",
			`{"company_id":7,"bank_name":"SBI"}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			id, ok := resolveVia(t, tc.method, tc.pattern, tc.target, tc.body)
			if !ok {
				t.Fatal("a request that names one company consistently was refused")
			}
			if id != 7 {
				t.Errorf("resolved company %d, expected 7", id)
			}
		})
	}
}

// A company id that cannot be read is not the same as none offered. Treating it as
// absent would let a second, unreadable value ride along beside a good one.
func TestCompanyForRequestRefusesUnreadableIdentifiers(t *testing.T) {
	if _, ok := resolveVia(t, http.MethodGet, "/api/v1/dashboard",
		"/api/v1/dashboard?companyId=notanumber", ""); ok {
		t.Error("an unreadable company id was accepted")
	}
}

// A route that needs a permission and offers no company at all has to fail closed.
func TestCompanyForRequestRefusesSilence(t *testing.T) {
	if _, ok := resolveVia(t, http.MethodPost, "/api/v1/items", "/api/v1/items", `{"name":"Tile"}`); ok {
		t.Error("a request naming no company was resolved")
	}
}

// The body is read whole and put back. A catalogue import is the big one, and reading
// only its first megabyte left truncated JSON that would not parse — which came back to
// the shop as "Unauthorized company access" for the sole offence of being large.
func TestCompanyFromBodyReadsPastAMegabyteAndRestoresIt(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Comfortably over the old 1 MiB peek, with the company id at the front where a
	// prefix read would have found it and the closing brace far beyond it.
	rows := strings.Repeat("Tile 600x600,XL-600,69072100,Box,1250.50,40\\n", 40000)
	body := `{"company_id":7,"csv":"Name,SKU,HSN,Unit,Price,Qty\\n` + rows + `"}`
	if len(body) <= 1<<20 {
		t.Fatalf("the fixture is only %d bytes; it has to exceed the old peek limit", len(body))
	}

	var (
		resolved int64
		ok       bool
		seen     int
	)
	r := gin.New()
	r.POST("/api/v1/items/import", func(c *gin.Context) {
		resolved, ok = companyForRequest(c, nil)
		// What the handler would read next has to be the whole body, not the remainder
		// after a peek and not an empty reader.
		rest, err := c.GetRawData()
		if err != nil {
			t.Errorf("the handler could not read the body back: %v", err)
		}
		seen = len(rest)
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/items/import", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(httptest.NewRecorder(), req)

	if !ok || resolved != 7 {
		t.Errorf("a large import resolved to (%d, %v); expected company 7", resolved, ok)
	}
	if seen != len(body) {
		t.Errorf("the handler got %d bytes of a %d byte body", seen, len(body))
	}
}

// Every route that touches what the shop spends needs the same answer. The list was
// governed and the single expense was not, so the counter refused the list could still
// read one expense by id and change its amount.
func TestExpenseRoutesAreAllGoverned(t *testing.T) {
	for _, route := range []string{
		"GET /api/v1/companies/:companyId/expenses",
		"GET /api/v1/companies/:companyId/expenses/summary",
		"POST /api/v1/expenses",
		"GET /api/v1/expenses/:id",
		"PUT /api/v1/expenses/:id",
		"DELETE /api/v1/expenses/:id",
	} {
		parts := strings.SplitN(route, " ", 2)
		if _, governed := RequiredPermission(parts[0], parts[1]); !governed {
			t.Errorf("%s is not in the permission table; staff would reach it", route)
		}
	}
}
