package services

import (
	"context"
	"database/sql"
	"time"

	"invo-server/internal/money"
)

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
	// Paid counts money handed over and goods sent back together: both reduce what the
	// shop owes, and a statement that showed only one would not add up.
	Paid    float64 `json:"paid"`
	Balance float64 `json:"balance"`
	Entries int     `json:"entries"`
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

	    UNION ALL

	    -- Goods sent back. A credit, because the supplier owes the shop for them:
	    -- whether that comes off the bill or sits as credit against the next one, the
	    -- statement has to show why the balance moved.
	    SELECT 'RETURN', r.id, r.return_date, r.created_at,
	           r.return_number,
	           'Returned — ' || r.return_number,
	           0::numeric, r.total
	    FROM purchase_returns r
	    WHERE r.company_id = $1 AND r.supplier_id = $2
	) entries
`

// SupplierLedger returns a supplier's statement, oldest first.
//
// A limit of 0 returns the whole history. With a limit, it is the newest page that
// comes back — still oldest-first within the page, the way a passbook reads — because
// what somebody wants on opening a statement is where it stands now, not where it began.
func (s *PurchaseService) SupplierLedger(
	ctx context.Context,
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

	rows, err := s.db.QueryContext(ctx, query, args...)
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
	ctx context.Context,
	companyID, supplierID int64,
) (SupplierLedgerSummary, error) {
	out := SupplierLedgerSummary{SupplierID: supplierID}

	err := s.db.QueryRowContext(ctx, `
		SELECT
			COALESCE((SELECT name FROM suppliers WHERE id = $2 AND company_id = $1), ''),
			COALESCE((
				SELECT SUM(total) FROM purchase_bills
				WHERE company_id = $1 AND supplier_id = $2 AND status <> 'cancelled'
			), 0),
			COALESCE((
				SELECT SUM(amount) FROM supplier_payments
				WHERE company_id = $1 AND supplier_id = $2
			), 0) + COALESCE((
				SELECT SUM(total) FROM purchase_returns
				WHERE company_id = $1 AND supplier_id = $2
			), 0),
			COALESCE((
				SELECT COUNT(*) FROM purchase_bills
				WHERE company_id = $1 AND supplier_id = $2 AND status <> 'cancelled'
			), 0) +
			COALESCE((
				SELECT COUNT(*) FROM supplier_payments
				WHERE company_id = $1 AND supplier_id = $2
			), 0) +
			COALESCE((
				SELECT COUNT(*) FROM purchase_returns
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

// SupplierStatementEntry is one line with a parsed time for PDF rendering.
type SupplierStatementEntry struct {
	Kind        string
	ID          int64
	Date        string
	EntryTime   time.Time
	Reference   string
	Description string
	Debit       float64
	Credit      float64
	Balance     float64
}

// SupplierStatement is a supplier's account over a period, ready for PDF rendering.
type SupplierStatement struct {
	CompanyName    string
	CompanyGSTIN   string
	CompanyPhone   string
	CompanyAddress string

	SupplierName    string
	SupplierPhone   string
	SupplierAddress string

	From    time.Time
	To      time.Time
	Opening float64
	Entries []SupplierStatementEntry
	Billed  float64
	Paid    float64
	Closing float64
}

// SupplierStatementFor builds a supplier's statement between two dates, inclusive.
//
// The opening balance is the running balance of the last entry before `from`. Entries
// in the period carry their balance as it stood at each moment — the full history window
// function runs inside the subquery, so a paged line's balance is still correct.
func (s *PurchaseService) SupplierStatementFor(
	ctx context.Context,
	companyID, supplierID int64,
	from, to time.Time,
) (SupplierStatement, error) {
	out := SupplierStatement{From: from, To: to}

	err := s.db.QueryRowContext(ctx, `
		SELECT
			COALESCE(co.name, ''), COALESCE(co.gst, ''), COALESCE(co.phone, ''),
			TRIM(BOTH ', ' FROM CONCAT_WS(', ',
				NULLIF(co.address, ''), NULLIF(co.city, ''),
				NULLIF(co.state, ''), NULLIF(co.pincode, ''))),
			COALESCE(su.name, ''), COALESCE(su.phone, ''),
			TRIM(BOTH ', ' FROM CONCAT_WS(', ',
				NULLIF(su.address, ''), NULLIF(su.city, ''),
				NULLIF(su.state, ''), NULLIF(su.pincode, '')))
		FROM suppliers su
		JOIN companies co ON co.id = su.company_id
		WHERE su.id = $1 AND su.company_id = $2
	`, supplierID, companyID).Scan(
		&out.CompanyName, &out.CompanyGSTIN, &out.CompanyPhone, &out.CompanyAddress,
		&out.SupplierName, &out.SupplierPhone, &out.SupplierAddress,
	)
	if err == sql.ErrNoRows {
		return out, PurchaseInputError{"That supplier isn't one of this company's."}
	}
	if err != nil {
		return out, err
	}

	// The balance at the moment the period opened — the last entry strictly before from.
	err = s.db.QueryRowContext(ctx, `
		SELECT COALESCE((
			SELECT balance FROM (`+supplierLedgerRows+`) history
			WHERE entry_date < $3
			ORDER BY entry_date DESC, entered_at DESC, kind DESC, id DESC
			LIMIT 1
		), 0)
	`, companyID, supplierID, from).Scan(&out.Opening)
	if err != nil {
		return out, err
	}

	// Entries in the period, oldest first, with their running balance from full history.
	end := to.AddDate(0, 0, 1)
	rows, err := s.db.QueryContext(ctx, `
		SELECT kind, id, date_text, entry_date, reference, description, debit, credit, balance
		FROM (`+supplierLedgerRows+`) history
		WHERE entry_date >= $3 AND entry_date < $4
		ORDER BY entry_date, entered_at, kind, id
	`, companyID, supplierID, from, end)
	if err != nil {
		return out, err
	}
	defer rows.Close()

	out.Entries = []SupplierStatementEntry{}
	for rows.Next() {
		var e SupplierStatementEntry
		if err := rows.Scan(
			&e.Kind, &e.ID, &e.Date, &e.EntryTime,
			&e.Reference, &e.Description,
			&e.Debit, &e.Credit, &e.Balance,
		); err != nil {
			return out, err
		}
		out.Billed += e.Debit
		out.Paid += e.Credit
		out.Entries = append(out.Entries, e)
	}
	if err := rows.Err(); err != nil {
		return out, err
	}

	if n := len(out.Entries); n > 0 {
		out.Closing = out.Entries[n-1].Balance
	} else {
		out.Closing = out.Opening
	}
	return out, nil
}
