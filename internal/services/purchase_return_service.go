package services

import (
	"context"
	"database/sql"
	"strings"

	"invo-server/internal/money"
)

// Sending stock back to a supplier.
//
// The mirror of a credit note. Tiles arrive broken or the wrong shade, the shop sends
// them back, and three things have to move together: the stock leaves the floor, what
// the shop owes that supplier comes down, and the return itself is recorded so the
// supplier's statement explains the change.
//
// Doing any one of those without the others is how the books stop matching the shed.

// PurchaseReturnLine is one item going back.
type PurchaseReturnLine struct {
	ItemID  int64   `json:"item_id"`
	Qty     int     `json:"qty"`
	Rate    float64 `json:"rate"`
	TaxRate float64 `json:"tax_rate"`
}

// PurchaseReturnRequest is a return as it is recorded.
type PurchaseReturnRequest struct {
	SupplierID   int64                `json:"supplier_id"`
	BillID       *int64               `json:"bill_id"`
	ReturnNumber string               `json:"return_number"`
	ReturnDate   *string              `json:"return_date"`
	Reason       string               `json:"reason"`
	Notes        string               `json:"notes"`
	Items        []PurchaseReturnLine `json:"items"`
}

// RecordReturn sends stock back and brings down what the supplier is owed.
//
// One transaction. A return that half-happened leaves a shop with stock it has sent
// away, or a supplier owed for goods sitting in their van.
func (s *PurchaseService) RecordReturn(
	ctx context.Context,
	companyID, userID int64,
	req PurchaseReturnRequest,
) (int64, error) {
	if strings.TrimSpace(req.ReturnNumber) == "" {
		return 0, PurchaseInputError{"Give this return a number."}
	}
	if len(req.Items) == 0 {
		return 0, PurchaseInputError{"A return needs at least one item."}
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	// Locked, like a bill: the supplier's balance moves here, and two returns at once
	// would otherwise each read it before the other wrote.
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

	// The bill, where one was named. It has to be this supplier's — returning against
	// somebody else's bill would move the credit to the wrong account.
	var billRemaining money.Amount
	if req.BillID != nil {
		var remaining float64
		var status string
		err = tx.QueryRowContext(ctx, `
			SELECT remaining_amount, status FROM purchase_bills
			WHERE id = $1 AND company_id = $2 AND supplier_id = $3
			FOR UPDATE
		`, *req.BillID, companyID, req.SupplierID).Scan(&remaining, &status)
		if err == sql.ErrNoRows {
			return 0, PurchaseInputError{"That bill isn't this supplier's."}
		}
		if err != nil {
			return 0, err
		}
		if status == "cancelled" {
			return 0, PurchaseInputError{"That bill has been cancelled."}
		}
		billRemaining = money.FromFloat(remaining)
	}

	subtotal, tax := money.Zero(), money.Zero()
	type returnLine struct {
		line  PurchaseReturnLine
		total money.Amount
	}
	lines := make([]returnLine, 0, len(req.Items))

	for _, line := range req.Items {
		if line.Qty <= 0 {
			return 0, PurchaseInputError{"Each line needs a quantity of at least one."}
		}
		if line.Rate < 0 {
			return 0, PurchaseInputError{"A rate cannot be negative."}
		}

		// Scoped to the company: an item id from the request is not trusted to belong
		// to this business just because the company id does.
		var itemOK bool
		var onHand int
		if err := tx.QueryRowContext(ctx,
			`SELECT TRUE, quantity FROM items WHERE id = $1 AND company_id = $2`,
			line.ItemID, companyID,
		).Scan(&itemOK, &onHand); err == sql.ErrNoRows {
			return 0, PurchaseInputError{"One of those items isn't in this company's catalogue."}
		} else if err != nil {
			return 0, err
		}

		// Sending back more than is on the floor would leave negative stock, which is
		// a figure that cannot be true and quietly breaks every count after it.
		if line.Qty > onHand {
			return 0, PurchaseInputError{
				"You don't have that many to send back — there are " +
					itoa(onHand) + " on hand.",
			}
		}

		// Against a named bill, no more can go back than came in on it, less whatever
		// has already been returned. Otherwise a bill could be returned twice over and
		// the supplier credited for goods they only ever sent once.
		if req.BillID != nil {
			var purchased, returned int
			if err := tx.QueryRowContext(ctx, `
				SELECT
					COALESCE((SELECT SUM(qty) FROM purchase_bill_items
					          WHERE bill_id = $1 AND item_id = $2), 0),
					COALESCE((SELECT SUM(ri.qty) FROM purchase_return_items ri
					          JOIN purchase_returns r ON r.id = ri.return_id
					          WHERE r.bill_id = $1 AND ri.item_id = $2), 0)
			`, *req.BillID, line.ItemID).Scan(&purchased, &returned); err != nil {
				return 0, err
			}
			if purchased == 0 {
				return 0, PurchaseInputError{"One of those items wasn't on that bill."}
			}
			if line.Qty+returned > purchased {
				return 0, PurchaseInputError{
					"That's more than came in on this bill — " +
						itoa(purchased-returned) + " left to return.",
				}
			}
		}

		net := money.FromFloat(line.Rate).MulQty(line.Qty).Round()
		lineTax := net.TaxAt(line.TaxRate)
		lines = append(lines, returnLine{line: line, total: net.Add(lineTax).Round()})
		subtotal = subtotal.Add(net)
		tax = tax.Add(lineTax)
	}

	total := subtotal.Add(tax).Round()

	var returnID int64
	err = tx.QueryRowContext(ctx, `
		INSERT INTO purchase_returns
			(company_id, user_id, supplier_id, bill_id, return_number, return_date,
			 subtotal, tax, total, reason, notes)
		VALUES ($1,$2,$3,$4,$5, COALESCE($6::date, CURRENT_DATE),$7,$8,$9,$10,$11)
		RETURNING id
	`,
		companyID, userID, req.SupplierID, req.BillID,
		strings.TrimSpace(req.ReturnNumber), req.ReturnDate,
		subtotal.Float64(), tax.Float64(), total.Float64(), req.Reason, req.Notes,
	).Scan(&returnID)
	if err != nil {
		if isDuplicateReturnNumber(err) {
			return 0, PurchaseInputError{"That return number is already used for this supplier."}
		}
		return 0, err
	}

	for _, l := range lines {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO purchase_return_items (return_id, item_id, qty, rate, tax_rate, total)
			VALUES ($1,$2,$3,$4,$5,$6)
		`, returnID, l.line.ItemID, l.line.Qty, l.line.Rate, l.line.TaxRate, l.total.Float64()); err != nil {
			return 0, err
		}

		var previous, updated int
		if err := tx.QueryRowContext(ctx, `
			UPDATE items
			SET quantity = quantity - $1, updated_at = NOW()
			WHERE id = $2 AND company_id = $3
			RETURNING quantity + $1, quantity
		`, l.line.Qty, l.line.ItemID, companyID).Scan(&previous, &updated); err != nil {
			return 0, err
		}

		if _, err := tx.ExecContext(ctx, `
			INSERT INTO stock_movements
				(item_id, company_id, user_id, movement_type, quantity_change,
				 previous_quantity, new_quantity, reference, note)
			VALUES ($1,$2,$3,'purchase_return',$4,$5,$6,$7,$8)
		`, l.line.ItemID, companyID, userID, -l.line.Qty, previous, updated,
			strings.TrimSpace(req.ReturnNumber), "Returned to supplier"); err != nil {
			return 0, err
		}
	}

	// The bill comes down by what went back, never below nothing. Anything beyond what
	// was still owed is money already handed over for goods now returned — that stays
	// with the supplier as credit, which the statement shows and the next bill spends.
	if req.BillID != nil {
		applied := total
		if applied.GreaterThan(billRemaining) {
			applied = billRemaining
		}
		if applied.GreaterThan(money.Zero()) {
			if _, err := tx.ExecContext(ctx, `
				UPDATE purchase_bills
				SET remaining_amount = remaining_amount - $2,
				    status = CASE WHEN remaining_amount - $2 <= 0 THEN 'paid' ELSE status END,
				    updated_at = NOW()
				WHERE id = $1
			`, *req.BillID, applied.Float64()); err != nil {
				return 0, err
			}
		}
	}

	return returnID, tx.Commit()
}

// isDuplicateReturnNumber matches the unique constraint by name rather than by the text
// of the message, which differs between Postgres versions.
func isDuplicateReturnNumber(err error) bool {
	return err != nil && strings.Contains(err.Error(), "purchase_returns_number_per_supplier")
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	negative := n < 0
	if negative {
		n = -n
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	if negative {
		return "-" + string(digits)
	}
	return string(digits)
}
