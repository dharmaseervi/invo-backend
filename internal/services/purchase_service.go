package services

import (
	"database/sql"
	"strings"

	"invo-server/internal/money"
)

// The buying side of a shop.
//
// Stock used to appear through a restock: a number typed in by hand, with no record of
// who it came from, what it cost, or whether it had been paid for. That left two
// questions a business asks constantly — what do I owe my suppliers, and what did this
// actually cost me — with no answer in the app at all.

// PurchaseInputError is a problem the person can fix, as opposed to a failure.
type PurchaseInputError struct{ Msg string }

func (e PurchaseInputError) Error() string { return e.Msg }

type PurchaseService struct {
	db *sql.DB
}

func NewPurchaseService(db *sql.DB) *PurchaseService {
	return &PurchaseService{db: db}
}

// PurchaseLine is one row of a supplier's bill.
type PurchaseLine struct {
	ItemID  int64   `json:"item_id"`
	Qty     int     `json:"qty"`
	Rate    float64 `json:"rate"`
	TaxRate float64 `json:"tax_rate"`
}

// PurchaseBillRequest is a bill as it is recorded.
type PurchaseBillRequest struct {
	SupplierID int64          `json:"supplier_id"`
	BillNumber string         `json:"bill_number"`
	BillDate   *string        `json:"bill_date"`
	DueDate    *string        `json:"due_date"`
	Notes      string         `json:"notes"`
	Items      []PurchaseLine `json:"items"`
	// What was paid at the counter, if anything. The rest becomes what the shop owes.
	PaidAmount float64 `json:"paid_amount"`
	PaidMethod string  `json:"paid_method"`
}

// RecordBill writes a supplier's bill: the stock arrives, the cost price is updated,
// and whatever was not paid becomes a due.
//
// One transaction, because a bill that half-arrived is worse than one that did not: the
// shop would have stock it cannot account for and a supplier balance that does not
// match the paperwork.
func (s *PurchaseService) RecordBill(
	companyID, userID int64,
	req PurchaseBillRequest,
) (int64, error) {
	if strings.TrimSpace(req.BillNumber) == "" {
		return 0, PurchaseInputError{"Enter the supplier's bill number."}
	}
	if len(req.Items) == 0 {
		return 0, PurchaseInputError{"A bill needs at least one item."}
	}
	if req.PaidAmount < 0 {
		return 0, PurchaseInputError{"Paid amount cannot be negative."}
	}

	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	var supplierOK bool
	if err := tx.QueryRow(
		`SELECT EXISTS(SELECT 1 FROM suppliers WHERE id = $1 AND company_id = $2)`,
		req.SupplierID, companyID,
	).Scan(&supplierOK); err != nil {
		return 0, err
	}
	if !supplierOK {
		return 0, PurchaseInputError{"That supplier isn't one of this company's."}
	}

	// Totals first, so the bill row is written once with the right figures.
	subtotal, tax := money.Zero(), money.Zero()
	type lineTotals struct {
		line  PurchaseLine
		net   money.Amount
		tax   money.Amount
		total money.Amount
	}
	lines := make([]lineTotals, 0, len(req.Items))

	for _, line := range req.Items {
		if line.Qty <= 0 {
			return 0, PurchaseInputError{"Each line needs a quantity of at least one."}
		}
		if line.Rate < 0 {
			return 0, PurchaseInputError{"A rate cannot be negative."}
		}
		if line.TaxRate < 0 || line.TaxRate > 100 {
			return 0, PurchaseInputError{"A GST rate must be between 0 and 100."}
		}

		// Scoped to the company: an item id from the request is not trusted to belong
		// to this business just because the company id does.
		var itemOK bool
		if err := tx.QueryRow(
			`SELECT EXISTS(SELECT 1 FROM items WHERE id = $1 AND company_id = $2)`,
			line.ItemID, companyID,
		).Scan(&itemOK); err != nil {
			return 0, err
		}
		if !itemOK {
			return 0, PurchaseInputError{"One of those items isn't in this company's catalogue."}
		}

		net := money.FromFloat(line.Rate).MulQty(line.Qty).Round()
		lineTax := net.TaxAt(line.TaxRate)
		lines = append(lines, lineTotals{line: line, net: net, tax: lineTax, total: net.Add(lineTax).Round()})
		subtotal = subtotal.Add(net)
		tax = tax.Add(lineTax)
	}

	total := subtotal.Add(tax).Round()
	paid := money.FromFloat(req.PaidAmount).Round()
	if paid.GreaterThan(total) {
		return 0, PurchaseInputError{"That's more than the bill comes to."}
	}
	remaining := total.Sub(paid).Round()

	status := "unpaid"
	switch {
	case remaining.IsZero():
		status = "paid"
	case paid.GreaterThan(money.Zero()):
		status = "partial"
	}

	var billID int64
	err = tx.QueryRow(`
		INSERT INTO purchase_bills (
			company_id, user_id, supplier_id, bill_number, bill_date, due_date,
			subtotal, tax, total, paid_amount, remaining_amount, status, notes
		)
		VALUES ($1,$2,$3,$4, COALESCE($5::date, CURRENT_DATE), $6::date, $7,$8,$9,$10,$11,$12,$13)
		RETURNING id
	`,
		companyID, userID, req.SupplierID, strings.TrimSpace(req.BillNumber),
		req.BillDate, req.DueDate,
		subtotal.Float64(), tax.Float64(), total.Float64(),
		paid.Float64(), remaining.Float64(), status, req.Notes,
	).Scan(&billID)

	if err != nil {
		if isDuplicateBillNumber(err) {
			return 0, PurchaseInputError{"That bill number is already recorded for this supplier."}
		}
		return 0, err
	}

	for _, l := range lines {
		if _, err := tx.Exec(`
			INSERT INTO purchase_bill_items (bill_id, item_id, qty, rate, tax_rate, total)
			VALUES ($1,$2,$3,$4,$5,$6)
		`, billID, l.line.ItemID, l.line.Qty, l.line.Rate, l.line.TaxRate, l.total.Float64()); err != nil {
			return 0, err
		}

		// Stock in, and the cost price follows the latest purchase.
		//
		// Latest rather than a weighted average: it is what a shopkeeper quotes when
		// asked what something costs, and it is the figure they can check against the
		// bill in their hand. An average is defensible too, but it cannot be found on
		// any piece of paper they have.
		var previous, updated int
		if err := tx.QueryRow(`
			UPDATE items
			SET quantity = quantity + $1,
			    cost_price = $2,
			    updated_at = NOW()
			WHERE id = $3 AND company_id = $4
			RETURNING quantity - $1, quantity
		`, l.line.Qty, l.line.Rate, l.line.ItemID, companyID).Scan(&previous, &updated); err != nil {
			return 0, err
		}

		if _, err := tx.Exec(`
			INSERT INTO stock_movements
				(item_id, company_id, user_id, movement_type, quantity_change,
				 previous_quantity, new_quantity, reference, note)
			VALUES ($1,$2,$3,'purchase',$4,$5,$6,$7,$8)
		`, l.line.ItemID, companyID, userID, l.line.Qty, previous, updated,
			strings.TrimSpace(req.BillNumber), "Purchase"); err != nil {
			return 0, err
		}
	}

	// Anything paid at the counter is recorded as a payment to the supplier, so the
	// payment history and the bill's balance cannot disagree.
	if paid.GreaterThan(money.Zero()) {
		method := strings.TrimSpace(req.PaidMethod)
		if method == "" {
			method = "cash"
		}
		if _, err := tx.Exec(`
			INSERT INTO supplier_payments (company_id, supplier_id, bill_id, amount, method, paid_on)
			VALUES ($1,$2,$3,$4,$5, COALESCE($6::date, CURRENT_DATE))
		`, companyID, req.SupplierID, billID, paid.Float64(), method, req.BillDate); err != nil {
			return 0, err
		}
	}

	return billID, tx.Commit()
}

// SupplierPaymentRequest is money going to a supplier.
type SupplierPaymentRequest struct {
	SupplierID int64   `json:"supplier_id"`
	BillID     *int64  `json:"bill_id"`
	Amount     float64 `json:"amount"`
	Method     string  `json:"method"`
	Reference  string  `json:"reference"`
	Notes      string  `json:"notes"`
	PaidOn     *string `json:"paid_on"`
}

// PaySupplier records a payment, against one bill or spread over the oldest unpaid ones.
//
// Oldest first, like the customer side: it is what a shop means by "paying off the
// account", and it keeps the ageing honest.
func (s *PurchaseService) PaySupplier(
	companyID int64,
	req SupplierPaymentRequest,
) (int64, error) {
	if req.Amount <= 0 {
		return 0, PurchaseInputError{"Enter how much was paid."}
	}
	if strings.TrimSpace(req.Method) == "" {
		return 0, PurchaseInputError{"Say how it was paid — cash, UPI, bank transfer."}
	}

	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	var supplierOK bool
	if err := tx.QueryRow(
		`SELECT EXISTS(SELECT 1 FROM suppliers WHERE id = $1 AND company_id = $2)`,
		req.SupplierID, companyID,
	).Scan(&supplierOK); err != nil {
		return 0, err
	}
	if !supplierOK {
		return 0, PurchaseInputError{"That supplier isn't one of this company's."}
	}

	remaining := money.FromFloat(req.Amount).Round()

	// The bills to settle: the one named, or every unpaid one oldest first. Locked, so
	// two payments at once cannot both read the same balance and overpay it.
	query := `
		SELECT id, remaining_amount FROM purchase_bills
		WHERE company_id = $1 AND supplier_id = $2 AND status IN ('unpaid', 'partial')
	`
	args := []interface{}{companyID, req.SupplierID}
	if req.BillID != nil {
		query += " AND id = $3"
		args = append(args, *req.BillID)
	}
	query += " ORDER BY bill_date ASC, id ASC FOR UPDATE"

	rows, err := tx.Query(query, args...)
	if err != nil {
		return 0, err
	}
	type due struct {
		id        int64
		remaining money.Amount
	}
	var dues []due
	for rows.Next() {
		var d due
		var amount float64
		if err := rows.Scan(&d.id, &amount); err != nil {
			rows.Close()
			return 0, err
		}
		d.remaining = money.FromFloat(amount)
		dues = append(dues, d)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}

	if req.BillID != nil && len(dues) == 0 {
		return 0, PurchaseInputError{"That bill isn't this supplier's, or it is already settled."}
	}

	for _, d := range dues {
		if !remaining.GreaterThan(money.Zero()) {
			break
		}
		applied := money.Min(remaining, d.remaining)
		if _, err := tx.Exec(`
			UPDATE purchase_bills
			SET paid_amount = paid_amount + $1,
			    remaining_amount = remaining_amount - $1,
			    status = CASE WHEN remaining_amount - $1 <= 0 THEN 'paid' ELSE 'partial' END,
			    updated_at = NOW()
			WHERE id = $2
		`, applied.Float64(), d.id); err != nil {
			return 0, err
		}
		remaining = remaining.Sub(applied)
	}

	// Anything left over is an advance to the supplier, recorded without a bill rather
	// than refused: paying ahead is ordinary, and refusing it would send somebody back
	// to a paper book.
	var paymentID int64
	if err := tx.QueryRow(`
		INSERT INTO supplier_payments
			(company_id, supplier_id, bill_id, amount, method, reference, notes, paid_on)
		VALUES ($1,$2,$3,$4,$5,$6,$7, COALESCE($8::date, CURRENT_DATE))
		RETURNING id
	`,
		companyID, req.SupplierID, req.BillID, req.Amount,
		strings.TrimSpace(req.Method), req.Reference, req.Notes, req.PaidOn,
	).Scan(&paymentID); err != nil {
		return 0, err
	}

	return paymentID, tx.Commit()
}

// isDuplicateBillNumber recognises the unique constraint on (supplier, bill number),
// which is worth turning into a sentence: it usually means the bill was entered twice.
//
// Matched on the constraint's name, which is what Postgres actually puts in the error —
// looking for "bill_number" found nothing, so this fell through to a generic failure
// and the one message that would have explained it never appeared.
func isDuplicateBillNumber(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "duplicate key") &&
		strings.Contains(msg, "purchase_bills_number_per_supplier")
}
