package services

import "invo-server/internal/money"

// A supplier's statement: every bill they sent, every payment made to them, and the
// running balance between the two. The buying-side twin of a customer's ledger.
//
// This is derived from the bills and the payments rather than kept as its own set of
// rows. A customer's balance is written as it goes because it is built up from things
// that happen in several places — invoices, payments, credit notes, adjustments. A
// supplier's is built from exactly two tables, and a third copy of the same arithmetic
// is how two figures that should agree stop agreeing. It also means the statement is
// right for bills recorded before it existed, with nothing to backfill.
//
// A positive balance means the shop owes the supplier, which is the way round a
// shopkeeper reads it: "I owe Kajaria ₹42,000". A negative one means the shop has paid
// ahead, which happens whenever a deposit is given against stock that has not arrived.

// SupplierLedgerEntry is one line of the statement.
type SupplierLedgerEntry struct {
	// "BILL" or "PAYMENT".
	Kind        string `json:"kind"`
	ID          int64  `json:"id"`
	Date        string `json:"date"`
	Reference   string `json:"reference"`
	Description string `json:"description"`
	// Debit is what the shop took on, credit is what it settled.
	Debit   float64 `json:"debit"`
	Credit  float64 `json:"credit"`
	Balance float64 `json:"balance"`
}

// SupplierLedgerSummary is a supplier's standing over their whole history.
type SupplierLedgerSummary struct {
	SupplierID int64   `json:"supplier_id"`
	Name       string  `json:"name"`
	Billed     float64 `json:"billed"`
	Paid       float64 `json:"paid"`
	Balance    float64 `json:"balance"`
	Entries    int     `json:"entries"`
}

// supplierLedgerRows is the statement as one ordered list with a running balance.
//
// The window runs over the same ordering the rows come back in, so every line's balance
// is what it was at that moment.
//
// Ordering is the date, then when the row was actually written, then kind and id. Bills
// and payments carry a date the person chose, and on a busy day several share it; the
// written-at time puts them back in the order they really happened, which is the order
// the balance column has to follow to make sense. The id at the end keeps a page from
// shifting under the reader.
//
// Cancelled bills are left out — the debt was never real — but a payment stays on the
// statement whatever became of the bill it was against, because the money did leave.
const supplierLedgerRows = `
	SELECT kind, id, entry_date, entered_at,
	       TO_CHAR(entry_date, 'YYYY-MM-DD') AS date_text,
	       reference, description, debit, credit,
	       SUM(debit - credit) OVER (
	           ORDER BY entry_date, entered_at, kind, id
	           ROWS BETWEEN UNBOUNDED PRECEDING AND CURRENT ROW
	       ) AS balance
	FROM (
	    SELECT 'BILL' AS kind, b.id, b.bill_date AS entry_date, b.created_at AS entered_at,
	           b.bill_number AS reference,
	           'Bill ' || b.bill_number AS description,
	           b.total AS debit, 0::numeric AS credit
	    FROM purchase_bills b
	    WHERE b.company_id = $1 AND b.supplier_id = $2 AND b.status <> 'cancelled'

	    UNION ALL

	    SELECT 'PAYMENT', p.id, p.paid_on, p.created_at,
	           COALESCE(p.reference, ''),
	           'Payment — ' || p.method,
	           0::numeric, p.amount
	    FROM supplier_payments p
	    WHERE p.company_id = $1 AND p.supplier_id = $2
	) entries
`

// SupplierLedger returns a supplier's statement, oldest first.
//
// A limit of 0 returns the whole history. With a limit, it is the newest page that
// comes back — still oldest-first within the page, the way a passbook reads — because
// what somebody wants on opening a statement is where it stands now, not where it began.
func (s *PurchaseService) SupplierLedger(
	companyID, supplierID int64,
	limit, offset int,
) ([]SupplierLedgerEntry, error) {
	// entry_date stays in the inner rows so the ordering can use a real date, while
	// the column the app reads is the plain YYYY-MM-DD every other endpoint returns.
	const columns = `kind, id, date_text, reference, description, debit, credit, balance`

	query := `SELECT ` + columns + ` FROM (` + supplierLedgerRows + `) history
		ORDER BY entry_date, entered_at, kind, id`
	args := []interface{}{companyID, supplierID}

	if limit > 0 {
		// Take the newest rows, then turn the page back the right way round. The
		// running balance is computed over the full history inside, so a paged line
		// still shows the balance as it stood — not a total restarted from the top
		// of the page.
		query = `SELECT ` + columns + ` FROM (
			SELECT * FROM (` + supplierLedgerRows + `) history
			ORDER BY entry_date DESC, entered_at DESC, kind DESC, id DESC
			LIMIT $3 OFFSET $4
		) page
		ORDER BY entry_date, entered_at, kind, id`
		args = append(args, limit, offset)
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []SupplierLedgerEntry{}
	for rows.Next() {
		var e SupplierLedgerEntry
		if err := rows.Scan(
			&e.Kind, &e.ID, &e.Date, &e.Reference, &e.Description,
			&e.Debit, &e.Credit, &e.Balance,
		); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// SupplierLedgerTotals is what the statement header shows: billed, paid, and what is
// still owed between them.
//
// It also serves as the ownership check for the statement: a supplier id belonging to
// another company has no name here, and the caller gets a refusal rather than an empty
// statement that looks like a supplier with no history.
func (s *PurchaseService) SupplierLedgerTotals(
	companyID, supplierID int64,
) (SupplierLedgerSummary, error) {
	out := SupplierLedgerSummary{SupplierID: supplierID}

	err := s.db.QueryRow(`
		SELECT
			COALESCE((SELECT name FROM suppliers WHERE id = $2 AND company_id = $1), ''),
			COALESCE((
				SELECT SUM(total) FROM purchase_bills
				WHERE company_id = $1 AND supplier_id = $2 AND status <> 'cancelled'
			), 0),
			COALESCE((
				SELECT SUM(amount) FROM supplier_payments
				WHERE company_id = $1 AND supplier_id = $2
			), 0),
			COALESCE((
				SELECT COUNT(*) FROM purchase_bills
				WHERE company_id = $1 AND supplier_id = $2 AND status <> 'cancelled'
			), 0) +
			COALESCE((
				SELECT COUNT(*) FROM supplier_payments
				WHERE company_id = $1 AND supplier_id = $2
			), 0)
	`, companyID, supplierID).Scan(&out.Name, &out.Billed, &out.Paid, &out.Entries)
	if err != nil {
		return out, err
	}
	if out.Name == "" {
		return out, PurchaseInputError{"That supplier isn't one of this company's."}
	}

	// Through the decimal arithmetic rather than straight float subtraction, which
	// turned ₹2,663.60 owing into 2663.5999999999985 on the way to the app.
	out.Balance = money.FromFloat(out.Billed).Sub(money.FromFloat(out.Paid)).Round().Float64()
	return out, nil
}
