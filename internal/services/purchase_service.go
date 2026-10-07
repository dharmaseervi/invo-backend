package services

import (
	"context"
	"database/sql"
	"math"
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
	// The invoice's final amount when no stock lines are being recorded. Mutually
	// exclusive with Items; nil preserves the existing item-based request format.
	BillAmount *float64 `json:"bill_amount"`
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
	ctx context.Context,
	companyID, userID int64,
	req PurchaseBillRequest,
) (int64, error) {
	if strings.TrimSpace(req.BillNumber) == "" {
		return 0, PurchaseInputError{"Enter the supplier's bill number."}
	}
	if req.BillAmount != nil {
		if len(req.Items) != 0 {
			return 0, PurchaseInputError{"Enter either an invoice amount or items, not both."}
		}
		amount := *req.BillAmount
		if math.IsNaN(amount) || math.IsInf(amount, 0) || amount <= 0 || amount > 9999999999.99 {
			return 0, PurchaseInputError{"Enter a valid invoice amount greater than zero."}
		}
		if money.FromFloat(amount).Round().IsZero() {
			return 0, PurchaseInputError{"The invoice amount must be at least 0.01."}
		}
	} else if len(req.Items) == 0 {
		return 0, PurchaseInputError{"Enter an invoice amount or add at least one item."}
	}
	if math.IsNaN(req.PaidAmount) || math.IsInf(req.PaidAmount, 0) || req.PaidAmount < 0 {
		return 0, PurchaseInputError{"Enter a valid paid amount of zero or more."}
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	// The supplier row is locked, not merely checked for existence. Two bills for the
	// same supplier arriving together both read whatever the shop has paid in advance,
	// and without the lock both would spend it. The lock sits on the supplier because
	// that is what the balance belongs to — the same reason a customer's ledger is
	// guarded by the client row.
	var supplierOK bool
	err = tx.QueryRowContext(ctx,
		`SELECT TRUE FROM suppliers WHERE id = $1 AND company_id = $2 FOR UPDATE`,
		req.SupplierID, companyID,
	).Scan(&supplierOK)
	if err == sql.ErrNoRows {
		return 0, PurchaseInputError{"That supplier isn't one of this company's."}
	} else if err != nil {
		return 0, err
	}

	// Totals first, so the bill row is written once with the right figures.
	subtotal, tax := money.Zero(), money.Zero()
	if req.BillAmount != nil {
		// The existing bill totals store the final amount without inventing a GST
		// split. Responses mark bills without lines as amount_only so clients do not
		// present this as an itemised or zero-rated tax invoice.
		subtotal = money.FromFloat(*req.BillAmount).Round()
	}
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
		if err := tx.QueryRowContext(ctx,
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

	// Money already paid to this supplier that no bill has claimed — an advance, or a
	// deposit against stock that had not arrived yet. It is spent on this bill now.
	//
	// Without this, two honest figures disagree: the statement shows the shop in credit
	// while the bill list shows this same bill unpaid, and nobody can say which to
	// believe. Settling the advance here keeps what each bill owes and what the supplier
	// is owed overall as two views of one number.
	credit, err := supplierCredit(ctx, tx, companyID, req.SupplierID)
	if err != nil {
		return 0, err
	}
	if credit.GreaterThan(money.Zero()) {
		applied := credit
		if applied.GreaterThan(total.Sub(paid)) {
			applied = total.Sub(paid).Round()
		}
		paid = paid.Add(applied).Round()
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
	err = tx.QueryRowContext(ctx, `
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
		if _, err := tx.ExecContext(ctx, `
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
		if err := tx.QueryRowContext(ctx, `
			UPDATE items
			SET quantity = quantity + $1,
			    cost_price = $2,
			    updated_at = NOW()
			WHERE id = $3 AND company_id = $4
			RETURNING quantity - $1, quantity
		`, l.line.Qty, l.line.Rate, l.line.ItemID, companyID).Scan(&previous, &updated); err != nil {
			return 0, err
		}

		if _, err := tx.ExecContext(ctx, `
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
	//
	// Only the money handed over now, never the advance applied above: that was
	// recorded as a payment when it was made, and writing it again would have the
	// statement claim the shop paid twice.
	if paidNow := money.FromFloat(req.PaidAmount).Round(); paidNow.GreaterThan(money.Zero()) {
		method := strings.TrimSpace(req.PaidMethod)
		if method == "" {
			method = "cash"
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO supplier_payments (company_id, supplier_id, bill_id, amount, method, paid_on)
			VALUES ($1,$2,$3,$4,$5, COALESCE($6::date, CURRENT_DATE))
		`, companyID, req.SupplierID, billID, paidNow.Float64(), method, req.BillDate); err != nil {
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
	ctx context.Context,
	companyID int64,
	req SupplierPaymentRequest,
) (int64, error) {
	if req.Amount <= 0 {
		return 0, PurchaseInputError{"Enter how much was paid."}
	}
	if strings.TrimSpace(req.Method) == "" {
		return 0, PurchaseInputError{"Say how it was paid — cash, UPI, bank transfer."}
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	// The supplier row is locked, not merely checked for existence. Two bills for the
	// same supplier arriving together both read whatever the shop has paid in advance,
	// and without the lock both would spend it. The lock sits on the supplier because
	// that is what the balance belongs to — the same reason a customer's ledger is
	// guarded by the client row.
	var supplierOK bool
	err = tx.QueryRowContext(ctx,
		`SELECT TRUE FROM suppliers WHERE id = $1 AND company_id = $2 FOR UPDATE`,
		req.SupplierID, companyID,
	).Scan(&supplierOK)
	if err == sql.ErrNoRows {
		return 0, PurchaseInputError{"That supplier isn't one of this company's."}
	} else if err != nil {
		return 0, err
	}

	remaining := money.FromFloat(req.Amount).Round()

	// The bills to settle: the one named, or every unpaid one oldest first.
	dues, err := openSupplierBills(ctx, tx, companyID, req.SupplierID, req.BillID, nil)
	if err != nil {
		return 0, err
	}
	if req.BillID != nil && len(dues) == 0 {
		return 0, PurchaseInputError{"That bill isn't this supplier's, or it is already settled."}
	}

	remaining, err = settleSupplierBills(ctx, tx, dues, remaining)
	if err != nil {
		return 0, err
	}

	// More than the named bill came to: the rest goes to this supplier's other open
	// bills, oldest first, instead of being set aside as credit.
	//
	// Left as credit it would read as a contradiction — the shop holding an advance
	// while a bill of theirs still showed unpaid — and the two figures a shopkeeper
	// checks, what each bill owes and what the supplier is owed, would stop agreeing.
	if req.BillID != nil && remaining.GreaterThan(money.Zero()) {
		others, err := openSupplierBills(ctx, tx, companyID, req.SupplierID, nil, req.BillID)
		if err != nil {
			return 0, err
		}
		if remaining, err = settleSupplierBills(ctx, tx, others, remaining); err != nil {
			return 0, err
		}
	}

	// Anything left over is an advance to the supplier, recorded without a bill rather
	// than refused: paying ahead is ordinary, and refusing it would send somebody back
	// to a paper book.
	var paymentID int64
	if err := tx.QueryRowContext(ctx, `
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

// supplierCredit is money paid to a supplier that no bill has claimed.
//
// Everything paid, less everything the bills account for. It is a derived figure rather
// than a stored balance: the advance is already recorded as a payment, and a second
// place to keep the same fact is a second place for it to go wrong.
func supplierCredit(ctx context.Context, tx *sql.Tx, companyID, supplierID int64) (money.Amount, error) {
	var paid, applied float64
	// Goods sent back count the same way money does: both are value the supplier owes
	// the shop. A return that the bills have not already absorbed is credit sitting
	// with them, and the next bill should spend it exactly as it spends an overpayment.
	//
	// What a bill has absorbed of a return is total - remaining - paid: a return reduces
	// what is still owed without pretending more money changed hands.
	err := tx.QueryRowContext(ctx, `
		SELECT
			COALESCE((
				SELECT SUM(amount) FROM supplier_payments
				WHERE company_id = $1 AND supplier_id = $2
			), 0) + COALESCE((
				SELECT SUM(total) FROM purchase_returns
				WHERE company_id = $1 AND supplier_id = $2
			), 0),
			COALESCE((
				SELECT SUM(paid_amount) FROM purchase_bills
				WHERE company_id = $1 AND supplier_id = $2 AND status <> 'cancelled'
			), 0) + COALESCE((
				SELECT SUM(total - remaining_amount - paid_amount) FROM purchase_bills
				WHERE company_id = $1 AND supplier_id = $2 AND status <> 'cancelled'
			), 0)
	`, companyID, supplierID).Scan(&paid, &applied)
	if err != nil {
		return money.Zero(), err
	}
	return money.FromFloat(paid).Sub(money.FromFloat(applied)).Round(), nil
}

// supplierDue is one open bill and what is still owed on it.
type supplierDue struct {
	id        int64
	remaining money.Amount
}

// openSupplierBills reads a supplier's unsettled bills, oldest first, and locks them.
//
// Locked because two payments arriving together would otherwise both read the same
// balance and both pay it off, leaving the supplier overpaid on paper.
//
// onlyBill restricts it to one bill; excludeBill leaves one out, for spreading what is
// left of a payment over everything else.
func openSupplierBills(
	ctx context.Context,
	tx *sql.Tx,
	companyID, supplierID int64,
	onlyBill, excludeBill *int64,
) ([]supplierDue, error) {
	query := `
		SELECT id, remaining_amount FROM purchase_bills
		WHERE company_id = $1 AND supplier_id = $2 AND status IN ('unpaid', 'partial')
	`
	args := []interface{}{companyID, supplierID}
	if onlyBill != nil {
		query += " AND id = $3"
		args = append(args, *onlyBill)
	} else if excludeBill != nil {
		query += " AND id <> $3"
		args = append(args, *excludeBill)
	}
	query += " ORDER BY bill_date ASC, id ASC FOR UPDATE"

	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var dues []supplierDue
	for rows.Next() {
		var d supplierDue
		var amount float64
		if err := rows.Scan(&d.id, &amount); err != nil {
			return nil, err
		}
		d.remaining = money.FromFloat(amount)
		dues = append(dues, d)
	}
	return dues, rows.Err()
}

// settleSupplierBills pays off bills in the order given and returns what is left of the
// money. Anything returned is an advance the supplier is holding.
func settleSupplierBills(
	ctx context.Context,
	tx *sql.Tx,
	dues []supplierDue,
	amount money.Amount,
) (money.Amount, error) {
	for _, d := range dues {
		if !amount.GreaterThan(money.Zero()) {
			break
		}
		applied := money.Min(amount, d.remaining)
		if _, err := tx.ExecContext(ctx, `
			UPDATE purchase_bills
			SET paid_amount = paid_amount + $1,
			    remaining_amount = remaining_amount - $1,
			    status = CASE WHEN remaining_amount - $1 <= 0 THEN 'paid' ELSE 'partial' END,
			    updated_at = NOW()
			WHERE id = $2
		`, applied.Float64(), d.id); err != nil {
			return amount, err
		}
		amount = amount.Sub(applied)
	}
	return amount, nil
}

// CancelBill voids a bill and reverses its stock in one transaction. Payments remain
// real money paid: freed credit settles other open bills, then remains as an advance.
func (s *PurchaseService) CancelBill(ctx context.Context, companyID, billID, userID int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// All supplier writes lock supplier before bill, including payments and returns.
	var supplierID int64
	err = tx.QueryRowContext(ctx, `SELECT supplier_id FROM purchase_bills WHERE id=$1 AND company_id=$2`, billID, companyID).Scan(&supplierID)
	if err == sql.ErrNoRows {
		return PurchaseInputError{"That bill isn't one of this company's."}
	}
	if err != nil {
		return err
	}
	var lockedID int64
	if err = tx.QueryRowContext(ctx, `SELECT id FROM suppliers WHERE id=$1 AND company_id=$2 FOR UPDATE`, supplierID, companyID).Scan(&lockedID); err != nil {
		return err
	}
	var status, reference string
	if err = tx.QueryRowContext(ctx, `SELECT status, bill_number FROM purchase_bills WHERE id=$1 AND company_id=$2 FOR UPDATE`, billID, companyID).Scan(&status, &reference); err != nil {
		return err
	}
	if status == "cancelled" {
		return PurchaseInputError{"That bill is already cancelled."}
	}
	var hasReturns bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM purchase_returns WHERE bill_id=$1)`, billID).Scan(&hasReturns); err != nil {
		return err
	}
	if hasReturns {
		return PurchaseInputError{"This bill has recorded returns and cannot be cancelled. Its stock and credit have already been adjusted."}
	}

	// Read and close the result before issuing writes on the transaction connection.
	rows, err := tx.QueryContext(ctx, `SELECT item_id, SUM(qty) FROM purchase_bill_items WHERE bill_id=$1 GROUP BY item_id ORDER BY item_id`, billID)
	if err != nil {
		return err
	}
	type stockLine struct {
		itemID int64
		qty    int
	}
	var lines []stockLine
	for rows.Next() {
		var line stockLine
		if err = rows.Scan(&line.itemID, &line.qty); err != nil {
			rows.Close()
			return err
		}
		lines = append(lines, line)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, line := range lines {
		var previous, updated int
		err = tx.QueryRowContext(ctx, `UPDATE items SET quantity=quantity-$1, updated_at=NOW()
   WHERE id=$2 AND company_id=$3 AND quantity >= $1 RETURNING quantity+$1, quantity`, line.qty, line.itemID, companyID).Scan(&previous, &updated)
		if err == sql.ErrNoRows {
			return PurchaseInputError{"There isn't enough stock to reverse this bill. Check sales, returns and stock adjustments first."}
		}
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO stock_movements
   (item_id, company_id, user_id, movement_type, quantity_change, previous_quantity, new_quantity, reference, note)
   VALUES ($1,$2,$3,'adjustment',$4,$5,$6,$7,'Purchase bill cancelled')`,
			line.itemID, companyID, userID, -line.qty, previous, updated, reference); err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, `UPDATE purchase_bills SET status='cancelled', paid_amount=0, remaining_amount=0, updated_at=NOW() WHERE id=$1`, billID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE supplier_payments SET bill_id=NULL WHERE bill_id=$1 AND company_id=$2`, billID, companyID); err != nil {
		return err
	}
	credit, err := supplierCredit(ctx, tx, companyID, supplierID)
	if err != nil {
		return err
	}
	if credit.GreaterThan(money.Zero()) {
		dues, err := openSupplierBills(ctx, tx, companyID, supplierID, nil, nil)
		if err != nil {
			return err
		}
		if _, err = settleSupplierBills(ctx, tx, dues, credit); err != nil {
			return err
		}
	}
	return tx.Commit()
}
