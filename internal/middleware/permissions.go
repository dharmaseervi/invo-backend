package middleware

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// Who may do what, in one place.
//
// The policy lives here as a table rather than as checks scattered through the handlers
// for two reasons: a question like "what can a counter boy actually do?" has to be
// answerable by reading something, and a permission that is enforced in fifteen places
// is a permission that is eventually missed in one of them.
//
// Anything not named in routePermissions needs membership of the company and nothing
// more. The sensitive routes are named, and a named route whose company cannot be
// worked out is refused rather than allowed.

// Role is what somebody is to a business.
type Role string

const (
	// RoleOwner is whoever the business belongs to: everything, including deciding
	// who else works here.
	RoleOwner Role = "owner"
	// RoleManager runs the shop day to day. Everything except changing the staff or
	// the company's own settings.
	RoleManager Role = "manager"
	// RoleStaff bills customers and takes payments. Not what things cost, not what
	// the shop earned, and nothing that deletes.
	RoleStaff Role = "staff"
)

// Permission is one thing a role may or may not do.
type Permission string

const (
	// PermBillCustomers covers writing invoices, estimates and taking payments —
	// the work of standing at the counter.
	PermBillCustomers Permission = "bill_customers"
	// PermEditCatalogue is changing items and their prices.
	PermEditCatalogue Permission = "edit_catalogue"
	// PermSeeCosts is anything that shows what the shop pays: purchases, suppliers,
	// cost prices, margins.
	PermSeeCosts Permission = "see_costs"
	// PermSeeReports is the money view of the business: takings, GST, ledgers, ageing.
	PermSeeReports Permission = "see_reports"
	// PermDelete is removing things that were already recorded.
	PermDelete Permission = "delete"
	// PermSettings is the company's own details, its banks and its invoice template.
	PermSettings Permission = "settings"
	// PermManageStaff is deciding who works here and what they may do.
	PermManageStaff Permission = "manage_staff"
)

// rolePermissions is the whole policy. Read down a column to see what a role can do.
var rolePermissions = map[Role]map[Permission]bool{
	RoleOwner: {
		PermBillCustomers: true,
		PermEditCatalogue: true,
		PermSeeCosts:      true,
		PermSeeReports:    true,
		PermDelete:        true,
		PermSettings:      true,
		PermManageStaff:   true,
	},
	RoleManager: {
		PermBillCustomers: true,
		PermEditCatalogue: true,
		PermSeeCosts:      true,
		PermSeeReports:    true,
		PermDelete:        true,
		// A manager runs the shop; they do not decide who works in it, and they
		// cannot change the GSTIN the shop bills under.
		PermSettings:    false,
		PermManageStaff: false,
	},
	RoleStaff: {
		PermBillCustomers: true,
		// Everything else is off. A counter boy billing a customer has no reason to
		// see what the tiles cost the shop, what it took this month, or to be able
		// to delete the invoice they just wrote.
		PermEditCatalogue: false,
		PermSeeCosts:      false,
		PermSeeReports:    false,
		PermDelete:        false,
		PermSettings:      false,
		PermManageStaff:   false,
	},
}

// Can reports whether a role may do something. An unknown role may do nothing, so a
// role name that is misspelled anywhere locks a door rather than opening one.
func Can(role Role, perm Permission) bool {
	return rolePermissions[role][perm]
}

// RoleCapabilities is a role's permissions as plain names, for an app that wants to
// hide what the person cannot use rather than let them find out by being refused.
func RoleCapabilities(role Role) map[string]bool {
	out := map[string]bool{}
	for perm, allowed := range rolePermissions[role] {
		out[string(perm)] = allowed
	}
	return out
}

// routePermissions names the routes that need more than membership, by method and the
// route's pattern as gin knows it.
//
// Patterns, not request paths: "/api/v1/invoices/:id" matches every invoice, and a new
// route that is not in this table is checked against the router at startup, so a
// sensitive route cannot be added and quietly left ungoverned.
var routePermissions = map[string]Permission{
	// Deleting things that were recorded.
	"DELETE /api/v1/invoices/:id":          PermDelete,
	"POST /api/v1/invoices/:id/cancel":     PermDelete,
	"DELETE /api/v1/clients/:clientId":     PermDelete,
	"DELETE /api/v1/expenses/:id":          PermDelete,
	"POST /api/v1/payments/:id/reverse":    PermDelete,
	"PUT /api/v1/payments/:id/allocations": PermDelete,

	// The catalogue and what things are priced at.
	"POST /api/v1/items":                PermEditCatalogue,
	"PUT /api/v1/items/:itemId":         PermEditCatalogue,
	"POST /api/v1/items/import":         PermEditCatalogue,
	"POST /api/v1/items/import/preview": PermEditCatalogue,
	"POST /api/v1/item/:itemId/restock": PermEditCatalogue,
	"POST /api/v1/categories":           PermEditCatalogue,

	// What the shop pays, and who it owes.
	"POST /api/v1/suppliers":                   PermSeeCosts,
	"GET /api/v1/suppliers":                    PermSeeCosts,
	"POST /api/v1/purchase-bills":              PermSeeCosts,
	"GET /api/v1/purchase-bills":               PermSeeCosts,
	"GET /api/v1/purchase-bills/:id":           PermSeeCosts,
	"POST /api/v1/supplier-payments":           PermSeeCosts,
	"POST /api/v1/purchase-returns":            PermSeeCosts,
	"GET /api/v1/purchase-returns":             PermSeeCosts,
	"GET /api/v1/suppliers/:id/ledger":         PermSeeCosts,
	"GET /api/v1/suppliers/:id/ledger/summary": PermSeeCosts,
	"GET /api/v1/item/:itemId/movements":       PermSeeCosts,

	// Closing the day. Counting the floor corrects stock and shows what everything
	// cost; counting the drawer is the day's takings. Neither is a counter job.
	"POST /api/v1/stocktakes":           PermEditCatalogue,
	"GET /api/v1/stocktakes/:id":        PermEditCatalogue,
	"POST /api/v1/stocktakes/:id/count": PermEditCatalogue,
	"POST /api/v1/stocktakes/:id/apply": PermEditCatalogue,
	"DELETE /api/v1/stocktakes/:id":     PermEditCatalogue,
	"GET /api/v1/day-closing":           PermSeeReports,
	"POST /api/v1/day-closing":          PermSeeReports,
	"GET /api/v1/day-closings":          PermSeeReports,

	// What the business earned and is owed.
	"GET /api/v1/ledger/:clientId":                      PermSeeReports,
	"GET /api/v1/ledger/:clientId/summary":              PermSeeReports,
	"GET /api/v1/ledger/:clientId/statement.pdf":        PermSeeReports,
	"GET /api/v1/companies/:companyId/ledger":           PermSeeReports,
	"GET /api/v1/companies/:companyId/ledger/summary":   PermSeeReports,
	"GET /api/v1/companies/:companyId/expenses":         PermSeeReports,
	"GET /api/v1/companies/:companyId/expenses/summary": PermSeeReports,
	// The list was governed and the single expense was not, so the counter who was
	// refused the expense list could still read any one expense by id and change its
	// amount. Recording one was open too. What the shop spends is one subject, and
	// every route that touches it needs the same answer.
	"POST /api/v1/expenses":    PermSeeReports,
	"GET /api/v1/expenses/:id": PermSeeReports,
	"PUT /api/v1/expenses/:id": PermSeeReports,
	"GET /api/v1/invoices/summary":                      PermSeeReports,
	"GET /api/v1/companies/:companyId/payments":         PermSeeReports,
	"GET /api/v1/dashboard":                             PermSeeReports,
	"GET /api/v1/companies/:companyId/reports/gstr1":    PermSeeReports,
	"GET /api/v1/companies/:companyId/reports/aging":    PermSeeReports,
	"GET /api/v1/companies/:companyId/reports/stock":    PermSeeReports,

	// The shop's own settings. Reading the bank list is left to any member: an
	// invoice prints those details, and somebody writing one may need to pick which
	// account it goes on. Changing them is the owner's.
	"PUT /api/v1/companies/:companyId":               PermSettings,
	"POST /api/v1/companies/:companyId/address":      PermSettings,
	"POST /api/v1/companies/:companyId/banks":        PermSettings,
	"PUT /api/v1/companies/:companyId/banks/:bankId": PermSettings,

	// Who works here.
	"GET /api/v1/companies/:companyId/staff":              PermManageStaff,
	"POST /api/v1/companies/:companyId/staff":             PermManageStaff,
	"PUT /api/v1/companies/:companyId/staff/:memberId":    PermManageStaff,
	"DELETE /api/v1/companies/:companyId/staff/:memberId": PermManageStaff,
}

// RequiredPermission returns what a route needs, and whether it needs anything at all.
func RequiredPermission(method, pattern string) (Permission, bool) {
	perm, ok := routePermissions[method+" "+pattern]
	return perm, ok
}

// GovernedRoutes is every route the policy names, for the startup check that none of
// them has been renamed out from under it.
func GovernedRoutes() []string {
	out := make([]string, 0, len(routePermissions))
	for route := range routePermissions {
		out = append(out, route)
	}
	return out
}

// Permissions enforces the policy above.
//
// It runs after authentication, so the caller is known; it works out which company the
// request concerns, reads that person's role in it, and refuses anything their role
// does not carry.
//
// A route the table does not name passes straight through — the handler's own
// membership check still applies, as it always did.
func Permissions(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		perm, governed := RequiredPermission(c.Request.Method, c.FullPath())
		if !governed {
			c.Next()
			return
		}

		userID := c.GetInt("user_id")
		companyID, ok := companyForRequest(c, db)
		if !ok {
			// The route needs a permission and we cannot tell which business it is
			// about. Refusing is the only safe answer: the alternative is letting an
			// unresolved request through the one check meant to stop it.
			c.JSON(http.StatusForbidden, gin.H{"error": "Unauthorized company access"})
			c.Abort()
			return
		}

		var role string
		err := db.QueryRow(
			`SELECT role FROM company_members WHERE company_id = $1 AND user_id = $2`,
			companyID, userID,
		).Scan(&role)

		switch {
		case err == sql.ErrNoRows:
			// Not a member at all. The handler would refuse this too; saying so here
			// keeps the message the same whichever check gets there first.
			c.JSON(http.StatusForbidden, gin.H{"error": "Unauthorized company access"})
			c.Abort()
			return
		case err != nil:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to verify access"})
			c.Abort()
			return
		}

		if !Can(Role(role), perm) {
			// Said plainly and without blame: the person has not done anything wrong,
			// their account simply does not carry this.
			c.JSON(http.StatusForbidden, gin.H{
				"error": "Your account doesn't have access to this. Ask the owner.",
			})
			c.Abort()
			return
		}

		c.Set("company_role", role)
		c.Next()
	}
}

// recordCompany says, for a route that names a record rather than a company, which
// table that record lives in and which path parameter holds its id.
//
// Keyed by the route pattern and not by the parameter's name, because ":id" means a
// different thing on nearly every route. Looking it up by name would have an expense id
// read as an invoice id — and the invoice that happens to share that number belongs to
// somebody else's shop.
var recordCompany = map[string]struct {
	param string
	query string
}{
	"/api/v1/invoices/:id":                   {"id", `SELECT company_id FROM invoices WHERE id = $1`},
	"/api/v1/invoices/:id/cancel":            {"id", `SELECT company_id FROM invoices WHERE id = $1`},
	"/api/v1/expenses/:id":                   {"id", `SELECT company_id FROM expensess WHERE id = $1`},
	"/api/v1/payments/:id/reverse":           {"id", `SELECT company_id FROM payments WHERE id = $1`},
	"/api/v1/payments/:id/allocations":       {"id", `SELECT company_id FROM payments WHERE id = $1`},
	"/api/v1/clients/:clientId":              {"clientId", `SELECT company_id FROM clients WHERE id = $1`},
	"/api/v1/ledger/:clientId":               {"clientId", `SELECT company_id FROM clients WHERE id = $1`},
	"/api/v1/ledger/:clientId/summary":       {"clientId", `SELECT company_id FROM clients WHERE id = $1`},
	"/api/v1/ledger/:clientId/statement.pdf": {"clientId", `SELECT company_id FROM clients WHERE id = $1`},
	"/api/v1/items/:itemId":                  {"itemId", `SELECT company_id FROM items WHERE id = $1`},
	"/api/v1/item/:itemId/restock":           {"itemId", `SELECT company_id FROM items WHERE id = $1`},
	"/api/v1/item/:itemId/movements":         {"itemId", `SELECT company_id FROM items WHERE id = $1`},
	"/api/v1/purchase-bills/:id":             {"id", `SELECT company_id FROM purchase_bills WHERE id = $1`},
	"/api/v1/suppliers/:id/ledger":           {"id", `SELECT company_id FROM suppliers WHERE id = $1`},
	"/api/v1/suppliers/:id/ledger/summary":   {"id", `SELECT company_id FROM suppliers WHERE id = $1`},
}

// companyForRequest works out which business a request concerns, and refuses to answer
// when the request names more than one.
//
// Taking the first identifier found was the hole. A request could say one company in
// the query string and another in its body: the check asked about the one you are an
// owner of, the handler then acted on the one you are only staff in, and a role that
// had been denied the catalogue could edit it. The id of a record belonging to somebody
// else plus the company_id of a shop you own aimed the same trick at a different
// target. A check that can be pointed somewhere other than the thing being done is not
// a check.
//
// So every identifier the request offers is collected and they must agree. Disagreement
// is refused rather than resolved in anyone's favour: there is no legitimate request
// that means two companies at once, and picking a winner is how the hole reopens.
func companyForRequest(c *gin.Context, db *sql.DB) (int64, bool) {
	var (
		resolved int64
		found    bool
	)

	// agree records one candidate. The first one sets the answer; every later one has
	// to match it.
	agree := func(id int64) bool {
		if !found {
			resolved, found = id, true
			return true
		}
		return id == resolved
	}

	// The record the route names, where there is one. This is the thing actually being
	// acted on, so it is the identifier that matters most — but it still only has to
	// agree with the others rather than silently overriding them.
	if lookup, ok := recordCompany[c.FullPath()]; ok {
		id, err := strconv.ParseInt(c.Param(lookup.param), 10, 64)
		if err != nil {
			return 0, false
		}
		var companyID int64
		if err := db.QueryRow(lookup.query, id).Scan(&companyID); err != nil {
			// Including a record that does not exist, which resolves to nothing and
			// is refused rather than waved through.
			return 0, false
		}
		if !agree(companyID) {
			return 0, false
		}
	}

	// Both spellings of both shapes. The API says company_id in most places and
	// companyId in others — the dashboard is one — and a resolver that knew only one
	// of them would refuse every request to the other for everybody, owners included.
	for _, v := range []string{
		c.Param("companyId"),
		c.Param("company_id"),
		c.Query("company_id"),
		c.Query("companyId"),
		c.GetHeader("X-Company-ID"),
	} {
		if v == "" {
			continue
		}
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			// An unreadable company id is not the same as none given. Ignoring it would
			// let "company_id=1&company_id=oops" through on whichever one parsed.
			return 0, false
		}
		if !agree(id) {
			return 0, false
		}
	}

	// And the JSON body. The catalogue import puts the company there and nowhere else,
	// so without this it resolved to nothing and every import was refused — which is
	// how a working feature broke the day this middleware was added. Failing closed is
	// right, but only when "closed" is not also closed to the people who should be let
	// through.
	if id, ok := companyFromBody(c); ok {
		if !agree(id) {
			return 0, false
		}
	}

	return resolved, found
}

// companyFromBody reads the JSON body looking for a company id and puts it back, so
// the handler still reads it whole.
//
// It reads all of it, not a prefix. Reading only the first megabyte meant a bigger
// catalogue import arrived as truncated JSON, which would not unmarshal, so the company
// resolved to nothing and the import was refused with "Unauthorized company access" —
// an authorisation error for a file that was simply large. The body is already bounded
// before this runs: the request is wrapped in a MaxBytesReader in main, so there is no
// limit to impose here beyond the one that is imposed anyway, and taking the whole
// thing is what makes the parse trustworthy.
func companyFromBody(c *gin.Context) (int64, bool) {
	if c.Request.Body == nil {
		return 0, false
	}

	body, err := io.ReadAll(c.Request.Body)
	// Whatever happens next — including a read that failed part-way, or a body over the
	// limit — the handler gets back what was read, so it reports the real problem
	// instead of seeing an empty body.
	c.Request.Body = io.NopCloser(bytes.NewReader(body))
	if err != nil {
		return 0, false
	}

	var probe struct {
		SnakeCase *json.Number `json:"company_id"`
		CamelCase *json.Number `json:"companyId"`
	}
	if err := json.Unmarshal(body, &probe); err != nil {
		return 0, false
	}

	for _, n := range []*json.Number{probe.SnakeCase, probe.CamelCase} {
		if n == nil {
			continue
		}
		if id, err := strconv.ParseInt(n.String(), 10, 64); err == nil {
			return id, true
		}
	}
	return 0, false
}
