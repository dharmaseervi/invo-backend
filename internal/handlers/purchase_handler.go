package handlers

import (
	"database/sql"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"

	"invo-server/internal/services"

	"github.com/gin-gonic/gin"
)

type PurchaseHandler struct {
	db      *sql.DB
	service *services.PurchaseService
}

func NewPurchaseHandler(db *sql.DB, service *services.PurchaseService) *PurchaseHandler {
	return &PurchaseHandler{db: db, service: service}
}

// company reads and authorises company_id from the query string.
func (h *PurchaseHandler) company(c *gin.Context) (int64, bool) {
	companyID, err := strconv.ParseInt(c.Query("company_id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "company_id is required"})
		return 0, false
	}
	owned, err := companyBelongsToUser(h.db, companyID, c.GetInt("user_id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to verify company"})
		return 0, false
	}
	if !owned {
		c.JSON(http.StatusForbidden, gin.H{"error": "Unauthorized company access"})
		return 0, false
	}
	return companyID, true
}

// fail turns a service error into the right status: a PurchaseInputError is something
// the person can fix and is reported word for word, anything else is a 500.
func fail(c *gin.Context, err error, generic string) {
	var input services.PurchaseInputError
	if errors.As(err, &input) {
		c.JSON(http.StatusBadRequest, gin.H{"error": input.Msg})
		return
	}
	log.Println(generic+":", err)
	c.JSON(http.StatusInternalServerError, gin.H{"error": generic})
}

// MARK: - Suppliers

// CreateSupplier adds somebody the shop buys from.
//
// POST /api/v1/suppliers?company_id=1
func (h *PurchaseHandler) CreateSupplier(c *gin.Context) {
	companyID, ok := h.company(c)
	if !ok {
		return
	}

	var req struct {
		Name    string `json:"name"`
		Phone   string `json:"phone"`
		Email   string `json:"email"`
		GSTIN   string `json:"gstin"`
		Address string `json:"address"`
		City    string `json:"city"`
		State   string `json:"state"`
		Pincode string `json:"pincode"`
		Notes   string `json:"notes"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid input"})
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "A supplier needs a name."})
		return
	}

	var id int64
	err := h.db.QueryRow(`
		INSERT INTO suppliers (company_id, user_id, name, phone, email, gstin, address, city, state, pincode, notes)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		RETURNING id
	`,
		companyID, c.GetInt("user_id"), strings.TrimSpace(req.Name), req.Phone, req.Email,
		strings.ToUpper(strings.TrimSpace(req.GSTIN)), req.Address, req.City, req.State, req.Pincode, req.Notes,
	).Scan(&id)

	if err != nil {
		log.Println("failed to create supplier:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save that supplier"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"message": "Supplier added", "supplier_id": id})
}

// GetSuppliers lists suppliers with what is owed to each.
//
// GET /api/v1/suppliers?company_id=1&search=&limit=&offset=
//
// The balance comes from the bills rather than being stored: a figure kept in two
// places is a figure that eventually disagrees with itself.
func (h *PurchaseHandler) GetSuppliers(c *gin.Context) {
	companyID, ok := h.company(c)
	if !ok {
		return
	}

	query := `
		SELECT s.id, s.name, COALESCE(s.phone, ''), COALESCE(s.email, ''),
		       COALESCE(s.gstin, ''), COALESCE(s.city, ''), COALESCE(s.state, ''),
		       COALESCE((
		           SELECT SUM(b.remaining_amount) FROM purchase_bills b
		           WHERE b.supplier_id = s.id AND b.status IN ('unpaid', 'partial')
		       ), 0),
		       COALESCE((
		           SELECT COUNT(*) FROM purchase_bills b
		           WHERE b.supplier_id = s.id AND b.status IN ('unpaid', 'partial')
		       ), 0),
		       -- Money paid to them that no bill has claimed: an advance they are
		       -- holding. Shown beside what is owed so the list and the statement say
		       -- the same thing — a supplier can be owed nothing and still be sitting
		       -- on a deposit, and a figure of zero alone hides that.
		       GREATEST(COALESCE((
		           SELECT SUM(p.amount) FROM supplier_payments p WHERE p.supplier_id = s.id
		       ), 0) - COALESCE((
		           SELECT SUM(b.paid_amount) FROM purchase_bills b
		           WHERE b.supplier_id = s.id AND b.status <> 'cancelled'
		       ), 0), 0)
		FROM suppliers s
		WHERE s.company_id = $1
	`
	args := []interface{}{companyID}
	if search := strings.TrimSpace(c.Query("search")); search != "" {
		query += ` AND (s.name ILIKE $2 OR COALESCE(s.phone, '') ILIKE $2)`
		args = append(args, "%"+search+"%")
	}
	query += " ORDER BY s.name, s.id"
	if limit := clampPageSize(mustAtoi(c.Query("limit")), 0); limit > 0 {
		query += " LIMIT $" + strconv.Itoa(len(args)+1) + " OFFSET $" + strconv.Itoa(len(args)+2)
		args = append(args, limit, mustAtoi(c.Query("offset")))
	}

	rows, err := h.db.Query(query, args...)
	if err != nil {
		log.Println("failed to fetch suppliers:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch suppliers"})
		return
	}
	defer rows.Close()

	out := []gin.H{}
	var totalDue, totalAdvance float64
	for rows.Next() {
		var (
			id                        int64
			name, phone, email, gstin string
			city, state               string
			due, advance              float64
			openBills                 int
		)
		if err := rows.Scan(&id, &name, &phone, &email, &gstin, &city, &state, &due, &openBills, &advance); err != nil {
			log.Println("failed to scan supplier:", err)
			continue
		}
		totalDue += due
		totalAdvance += advance
		out = append(out, gin.H{
			"id": id, "name": name, "phone": phone, "email": email, "gstin": gstin,
			"city": city, "state": state, "due": due, "open_bills": openBills,
			"advance": advance,
		})
	}

	c.JSON(http.StatusOK, gin.H{"data": out, "total_due": totalDue, "total_advance": totalAdvance})
}

// MARK: - Bills

// RecordBill writes a supplier's bill: stock in, cost updated, dues raised.
//
// POST /api/v1/purchase-bills?company_id=1
func (h *PurchaseHandler) RecordBill(c *gin.Context) {
	companyID, ok := h.company(c)
	if !ok {
		return
	}

	var req services.PurchaseBillRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid input"})
		return
	}

	id, err := h.service.RecordBill(c.Request.Context(), companyID, int64(c.GetInt("user_id")), req)
	if err != nil {
		fail(c, err, "Failed to record that bill")
		return
	}

	c.JSON(http.StatusCreated, gin.H{"message": "Bill recorded", "bill_id": id})
}

// GetBills lists purchase bills, newest first.
//
// GET /api/v1/purchase-bills?company_id=1&supplier_id=&status=&limit=&offset=
func (h *PurchaseHandler) GetBills(c *gin.Context) {
	companyID, ok := h.company(c)
	if !ok {
		return
	}

	query := `
		SELECT b.id, b.supplier_id, COALESCE(s.name, ''), b.bill_number,
		       TO_CHAR(b.bill_date, 'YYYY-MM-DD'),
		       COALESCE(TO_CHAR(b.due_date, 'YYYY-MM-DD'), ''),
		       b.subtotal, b.tax, b.total, b.paid_amount, b.remaining_amount, b.status,
		       (b.due_date IS NOT NULL AND CURRENT_DATE > b.due_date
		        AND b.status IN ('unpaid','partial')) AS is_overdue
		FROM purchase_bills b
		JOIN suppliers s ON s.id = b.supplier_id
		WHERE b.company_id = $1
	`
	args := []interface{}{companyID}
	if v := c.Query("supplier_id"); v != "" {
		if supplierID, err := strconv.ParseInt(v, 10, 64); err == nil {
			query += " AND b.supplier_id = $" + strconv.Itoa(len(args)+1)
			args = append(args, supplierID)
		}
	}
	switch strings.ToLower(strings.TrimSpace(c.Query("status"))) {
	case "unpaid":
		query += " AND b.status = 'unpaid'"
	case "partial":
		query += " AND b.status = 'partial'"
	case "paid":
		query += " AND b.status = 'paid'"
	case "owed":
		query += " AND b.status IN ('unpaid','partial')"
	case "overdue":
		query += " AND b.status IN ('unpaid','partial') AND b.due_date IS NOT NULL AND CURRENT_DATE > b.due_date"
	}
	query += " ORDER BY b.bill_date DESC, b.id DESC"
	if limit := clampPageSize(mustAtoi(c.DefaultQuery("limit", "50")), 50); limit > 0 {
		query += " LIMIT $" + strconv.Itoa(len(args)+1) + " OFFSET $" + strconv.Itoa(len(args)+2)
		args = append(args, limit, mustAtoi(c.Query("offset")))
	}

	rows, err := h.db.Query(query, args...)
	if err != nil {
		log.Println("failed to fetch purchase bills:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch bills"})
		return
	}
	defer rows.Close()

	out := []gin.H{}
	for rows.Next() {
		var (
			id, supplierID                              int64
			supplierName, billNumber, billDate, dueDate string
			subtotal, tax, total, paid, remaining       float64
			status                                      string
			isOverdue                                   bool
		)
		if err := rows.Scan(&id, &supplierID, &supplierName, &billNumber, &billDate, &dueDate,
			&subtotal, &tax, &total, &paid, &remaining, &status, &isOverdue); err != nil {
			log.Println("failed to scan purchase bill:", err)
			continue
		}
		out = append(out, gin.H{
			"id": id, "supplier_id": supplierID, "supplier_name": supplierName,
			"bill_number": billNumber, "bill_date": billDate, "due_date": dueDate,
			"subtotal": subtotal, "tax": tax, "total": total,
			"paid_amount": paid, "remaining_amount": remaining,
			"status": status, "is_overdue": isOverdue,
		})
	}

	c.JSON(http.StatusOK, gin.H{"data": out})
}

// GetBill returns one bill with its lines.
//
// GET /api/v1/purchase-bills/:id?company_id=1
func (h *PurchaseHandler) GetBill(c *gin.Context) {
	companyID, ok := h.company(c)
	if !ok {
		return
	}
	billID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid bill id"})
		return
	}

	var (
		supplierID                                  int64
		supplierName, billNumber, billDate, dueDate string
		subtotal, tax, total, paid, remaining       float64
		status, notes                               string
	)
	err = h.db.QueryRow(`
		SELECT b.supplier_id, COALESCE(s.name, ''), b.bill_number,
		       TO_CHAR(b.bill_date, 'YYYY-MM-DD'),
		       COALESCE(TO_CHAR(b.due_date, 'YYYY-MM-DD'), ''),
		       b.subtotal, b.tax, b.total, b.paid_amount, b.remaining_amount,
		       b.status, COALESCE(b.notes, '')
		FROM purchase_bills b
		JOIN suppliers s ON s.id = b.supplier_id
		WHERE b.id = $1 AND b.company_id = $2
	`, billID, companyID).Scan(&supplierID, &supplierName, &billNumber, &billDate, &dueDate,
		&subtotal, &tax, &total, &paid, &remaining, &status, &notes)

	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Bill not found"})
		return
	}
	if err != nil {
		log.Println("failed to fetch purchase bill:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch that bill"})
		return
	}

	rows, err := h.db.Query(`
		SELECT bi.item_id, COALESCE(i.name, ''), bi.qty, bi.rate, bi.tax_rate, bi.total
		FROM purchase_bill_items bi
		LEFT JOIN items i ON i.id = bi.item_id
		WHERE bi.bill_id = $1
		ORDER BY bi.id
	`, billID)
	if err != nil {
		log.Println("failed to fetch purchase bill items:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch that bill"})
		return
	}
	defer rows.Close()

	items := []gin.H{}
	for rows.Next() {
		var (
			itemID                   int64
			name                     string
			qty                      int
			rate, taxRate, lineTotal float64
		)
		if err := rows.Scan(&itemID, &name, &qty, &rate, &taxRate, &lineTotal); err != nil {
			continue
		}
		items = append(items, gin.H{
			"item_id": itemID, "item_name": name, "qty": qty,
			"rate": rate, "tax_rate": taxRate, "total": lineTotal,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"id": billID, "supplier_id": supplierID, "supplier_name": supplierName,
		"bill_number": billNumber, "bill_date": billDate, "due_date": dueDate,
		"subtotal": subtotal, "tax": tax, "total": total,
		"paid_amount": paid, "remaining_amount": remaining,
		"status": status, "notes": notes, "items": items,
	})
}

// MARK: - Paying suppliers

// PaySupplier records money paid to a supplier.
//
// POST /api/v1/supplier-payments?company_id=1
func (h *PurchaseHandler) PaySupplier(c *gin.Context) {
	companyID, ok := h.company(c)
	if !ok {
		return
	}

	var req services.SupplierPaymentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid input"})
		return
	}

	id, err := h.service.PaySupplier(c.Request.Context(), companyID, req)
	if err != nil {
		fail(c, err, "Failed to record that payment")
		return
	}

	c.JSON(http.StatusCreated, gin.H{"message": "Payment recorded", "payment_id": id})
}

// MARK: - Supplier statement

// SupplierLedger returns one supplier's statement: their bills, the payments made to
// them, and the running balance.
//
// GET /api/v1/suppliers/:id/ledger?company_id=1&limit=&offset=
//
// The summary comes back with the entries rather than only from the summary endpoint,
// so a statement can be drawn from one request. A statement that shows a balance from
// one call and lines from another can disagree with itself while the second is still
// in the air.
func (h *PurchaseHandler) SupplierLedger(c *gin.Context) {
	companyID, ok := h.company(c)
	if !ok {
		return
	}
	supplierID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid supplier"})
		return
	}

	// Totals first: it is also the check that this supplier is the company's, and
	// there is no sense reading a statement we are about to refuse.
	summary, err := h.service.SupplierLedgerTotals(c.Request.Context(), companyID, supplierID)
	if err != nil {
		fail(c, err, "Failed to load that statement")
		return
	}

	entries, err := h.service.SupplierLedger(
		c.Request.Context(), companyID, supplierID,
		clampPageSize(mustAtoi(c.Query("limit")), 0),
		mustAtoi(c.Query("offset")),
	)
	if err != nil {
		fail(c, err, "Failed to load that statement")
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": entries, "summary": summary})
}

// SupplierLedgerSummary returns just where a supplier stands, for a screen that shows
// the figure without the lines behind it.
//
// GET /api/v1/suppliers/:id/ledger/summary?company_id=1
func (h *PurchaseHandler) SupplierLedgerSummary(c *gin.Context) {
	companyID, ok := h.company(c)
	if !ok {
		return
	}
	supplierID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid supplier"})
		return
	}

	summary, err := h.service.SupplierLedgerTotals(c.Request.Context(), companyID, supplierID)
	if err != nil {
		fail(c, err, "Failed to load that statement")
		return
	}
	c.JSON(http.StatusOK, summary)
}
