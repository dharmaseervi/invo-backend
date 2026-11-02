package middleware

import (
	"github.com/gin-gonic/gin"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

// Parse actual registrations rather than duplicating the route inventory in a test.
// This catches new public/protected endpoints, renamed endpoints and stale exceptions.
func TestEveryRegisteredRouteHasReviewedPolicy(t *testing.T) {
	f, err := parser.ParseFile(token.NewFileSet(), "../routes/route.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	registered := map[string]bool{}
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || len(call.Args) == 0 {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if !strings.Contains(" GET POST PUT PATCH DELETE ", " "+sel.Sel.Name+" ") {
			return true
		}
		group, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}
		prefix := "/api/v1"
		switch group.Name {
		case "protected", "public", "credentials":
		case "r":
			prefix = ""
		default:
			return true
		}
		lit, ok := call.Args[0].(*ast.BasicLit)
		if !ok {
			return true
		}
		path, err := strconv.Unquote(lit.Value)
		if err != nil {
			t.Fatal(err)
		}
		key := sel.Sel.Name + " " + prefix + path
		registered[key] = true
		_, governed := RequiredPermission(sel.Sel.Name, prefix+path)
		reason, exempt := RouteExemption(sel.Sel.Name, prefix+path)
		if governed == exempt {
			t.Errorf("%s needs exactly one policy or exemption", key)
		}
		if exempt && reason == "" {
			t.Errorf("%s needs an exemption reason", key)
		}
		return true
	})
	for _, key := range ReviewedRoutes() {
		if !registered[key] {
			t.Errorf("stale policy %s", key)
		}
	}
}
func TestUnreviewedRoutesFailClosed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(Permissions(nil))
	r.GET("/api/v1/future-report", func(c *gin.Context) { c.Status(200) })
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/future-report", nil))
	if w.Code != 403 {
		t.Fatalf("got %d", w.Code)
	}
}
func TestCostRoutesRequireCostPermission(t *testing.T) {
	for _, path := range []string{"/api/v1/companies/:companyId/reports/supplier-aging", "/api/v1/purchase-returns/:id/pdf"} {
		perm, ok := RequiredPermission("GET", path)
		if !ok || perm != PermSeeCosts {
			t.Fatalf("missing cost policy %s", path)
		}
		if Can(RoleStaff, perm) || !Can(RoleOwner, perm) || !Can(RoleManager, perm) {
			t.Fatal("unexpected role policy")
		}
	}
}
