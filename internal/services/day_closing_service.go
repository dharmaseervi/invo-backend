package services

import (
	"database/sql"
	"encoding/json"
	"strings"

	"invo-server/internal/money"
)

// Counting the cash drawer at the end of the day.
//
// What a till holds is what was in it this morning, plus what came in, less what went
// out of it. All three matter: a shop pays its suppliers, refunds customers and buys tea
// out of the same drawer, and it starts the day with a float that was never takings.
//
// Counting receipts alone — which is what this did first — tells a shop it is short by
// everything it paid out and over by its own float, every day, until somebody stops
// believing the figure. A number nobody trusts is worse than no number.

// CashLine is one component of the day's drawer, so a difference can be chased rather
// than stared at.
type CashLine struct {
	Label  string  `json:"label"`
	Amount float64 `json:"amount"`
	Count  int     `json:"count"`
}

// DayClosing is one day's cash: what the drawer should hold against what was counted.
type DayClosing struct {
	Date string `json:"date"`

	// What was in the drawer before the day started.
	Opening float64 `json:"opening_cash"`
	// Cash taken in, and cash paid out of the same drawer.
	CashIn  float64 `json:"cash_in"`
	CashOut float64 `json:"cash_out"`
	// Opening + in - out: what should be there.
	Expected float64 `json:"expected_cash"`
	// What was actually there.
	Counted float64 `json:"counted_cash"`
	// Counted less expected: negative is short, positive is over.
	Difference float64 `json:"difference"`

	Note   string `json:"note"`
	Closed bool   `json:"closed"`

	// Where the two sides came from.
	InBreakdown  []CashLine `json:"in_breakdown"`
	OutBreakdown []CashLine `json:"out_breakdown"`
}

// cashMethods is what counts as cash, in the several spellings the apps have used.
const cashMethods = `('cash', 'cash payment')`

// drawerFor works out what the drawer should hold for a date, and where each side of it
// came from.
//
// Reversed payments are left out of what came in, because the money went back out
// again — counting them would have the till look short by exactly the amount handed
// back. Expenses count only where they were marked as paid in cash: an expense with no
// method recorded is an unknown, and treating unknowns as cash would take money out of
// the drawer figure that may never have left it.
func (s *LedgerService) drawerFor(companyID int64, date string) (
	opening, cashIn, cashOut float64,
	inLines, outLines []CashLine,
	err error,
) {
	// Started empty rather than nil, so a quiet day serialises as [] and not null —
	// an app decoding a list does not expect the list itself to go missing.
	inLines, outLines = []CashLine{}, []CashLine{}

	// What yesterday's count left behind. The drawer carries over; a shop does not
	// empty it to the rupee every night.
	err = s.db.QueryRow(`
		SELECT COALESCE((
			SELECT counted_cash FROM day_closings
			WHERE company_id = $1 AND closing_date < $2::date
			ORDER BY closing_date DESC LIMIT 1
		), 0)
	`, companyID, date).Scan(&opening)
	if err != nil {
		return
	}

	// An opening float that was set by hand on this day's own closing wins: somebody
	// counted the drawer this morning and said so.
	var storedOpening sql.NullFloat64
	err = s.db.QueryRow(`
		SELECT opening_cash FROM day_closings
		WHERE company_id = $1 AND closing_date = $2::date
	`, companyID, date).Scan(&storedOpening)
	switch {
	case err == sql.ErrNoRows:
		err = nil
	case err != nil:
		return
	case storedOpening.Valid:
		opening = storedOpening.Float64
	}

	add := func(lines *[]CashLine, total *float64, label, query string) error {
		var amount sql.NullFloat64
		var count int
		if scanErr := s.db.QueryRow(query, companyID, date).Scan(&amount, &count); scanErr != nil {
			return scanErr
		}
		if count == 0 {
			return nil
		}
		*total += amount.Float64
		*lines = append(*lines, CashLine{Label: label, Amount: amount.Float64, Count: count})
		return nil
	}

	if err = add(&inLines, &cashIn, "Payments received", `
		SELECT COALESCE(SUM(amount), 0), COUNT(*) FROM payments
		WHERE company_id = $1 AND payment_date = $2::date
		  AND status <> 'reversed'
		  AND lower(COALESCE(payment_method, '')) IN `+cashMethods); err != nil {
		return
	}

	if err = add(&outLines, &cashOut, "Paid to suppliers", `
		SELECT COALESCE(SUM(amount), 0), COUNT(*) FROM supplier_payments
		WHERE company_id = $1 AND paid_on = $2::date
		  AND lower(COALESCE(method, '')) IN `+cashMethods); err != nil {
		return
	}

	if err = add(&outLines, &cashOut, "Refunded to customers", `
		SELECT COALESCE(SUM(amount), 0), COUNT(*) FROM refunds
		WHERE company_id = $1 AND refund_date = $2::date
		  AND lower(COALESCE(method, '')) IN `+cashMethods); err != nil {
		return
	}

	if err = add(&outLines, &cashOut, "Expenses paid in cash", `
		SELECT COALESCE(SUM(amount), 0), COUNT(*) FROM expensess
		WHERE company_id = $1 AND date = $2::date
		  AND lower(COALESCE(payment_method, '')) IN `+cashMethods); err != nil {
		return
	}

	return
}

// ClosingFor returns the recorded snapshot for closed days. Only open days use live
// transactions, so later corrections cannot change one side of a saved reconciliation.
func (s *LedgerService) ClosingFor(companyID int64, date string) (DayClosing, error) {
	out := DayClosing{Date: date}
	var inJSON, outJSON []byte
	err := s.db.QueryRow(`
		SELECT opening_cash, cash_in, cash_out, expected_cash, counted_cash,
		       difference, COALESCE(note, ''), in_breakdown, out_breakdown
		FROM day_closings
		WHERE company_id = $1 AND closing_date = $2::date
	`, companyID, date).Scan(
		&out.Opening, &out.CashIn, &out.CashOut, &out.Expected, &out.Counted,
		&out.Difference, &out.Note, &inJSON, &outJSON,
	)
	if err == nil {
		out.InBreakdown, err = closingBreakdown(inJSON, "Cash in at closing", out.CashIn)
		if err != nil {
			return DayClosing{}, err
		}
		out.OutBreakdown, err = closingBreakdown(outJSON, "Cash out at closing", out.CashOut)
		if err != nil {
			return DayClosing{}, err
		}
		out.Closed = true
		return out, nil
	}
	if err != sql.ErrNoRows {
		return DayClosing{}, err
	}

	opening, cashIn, cashOut, inLines, outLines, err := s.drawerFor(companyID, date)
	if err != nil {
		return DayClosing{}, err
	}

	expected := money.FromFloat(opening).
		Add(money.FromFloat(cashIn)).
		Sub(money.FromFloat(cashOut)).
		Round()

	out = DayClosing{
		Date:         date,
		Opening:      opening,
		CashIn:       cashIn,
		CashOut:      cashOut,
		Expected:     expected.Float64(),
		InBreakdown:  inLines,
		OutBreakdown: outLines,
	}

	return out, nil
}

// Older closings have saved totals but no saved category breakdown. Show those totals
// without fabricating transaction counts or reconstructing history from today's data.
func closingBreakdown(data []byte, label string, total float64) ([]CashLine, error) {
	if len(data) == 0 || string(data) == "null" {
		if total == 0 {
			return []CashLine{}, nil
		}
		return []CashLine{{Label: label, Amount: total}}, nil
	}
	var lines []CashLine
	if err := json.Unmarshal(data, &lines); err != nil {
		return nil, err
	}
	return lines, nil
}

// Close records what was counted out of the drawer.
//
// An opening float can be given where it is not simply what yesterday left behind — a
// shop that tops the drawer up each morning, or one closing its very first day.
//
// Every component is stored, not just the difference: a payment corrected next week
// would otherwise silently change what last Tuesday's closing appeared to have found. A
// closing is a record of what was true when somebody counted.
//
// Closing the same day twice replaces the figure rather than adding a second one —
// recounting is correcting the first count, not a separate event.
func (s *LedgerService) Close(
	companyID, userID int64,
	date string,
	counted float64,
	opening *float64,
	note string,
) (DayClosing, error) {
	if counted < 0 {
		return DayClosing{}, LedgerInputError{"A cash count cannot be negative."}
	}
	if opening != nil && *opening < 0 {
		return DayClosing{}, LedgerInputError{"Opening cash cannot be negative."}
	}

	carried, cashIn, cashOut, inLines, outLines, err := s.drawerFor(companyID, date)
	if err != nil {
		return DayClosing{}, err
	}
	if opening != nil {
		carried = *opening
	}

	expected := money.FromFloat(carried).
		Add(money.FromFloat(cashIn)).
		Sub(money.FromFloat(cashOut)).
		Round()
	difference := money.FromFloat(counted).Sub(expected).Round()
	inJSON, err := json.Marshal(inLines)
	if err != nil {
		return DayClosing{}, err
	}
	outJSON, err := json.Marshal(outLines)
	if err != nil {
		return DayClosing{}, err
	}

	_, err = s.db.Exec(`
		INSERT INTO day_closings
			(company_id, user_id, closing_date, opening_cash, cash_in, cash_out,
			 expected_cash, counted_cash, difference, note, in_breakdown, out_breakdown)
		VALUES ($1, $2, $3::date, $4, $5, $6, $7, $8, $9, $10, $11::jsonb, $12::jsonb)
		ON CONFLICT (company_id, closing_date)
		DO UPDATE SET opening_cash  = EXCLUDED.opening_cash,
		              cash_in       = EXCLUDED.cash_in,
		              cash_out      = EXCLUDED.cash_out,
		              expected_cash = EXCLUDED.expected_cash,
		              counted_cash  = EXCLUDED.counted_cash,
		              difference    = EXCLUDED.difference,
		              note          = EXCLUDED.note,
		              in_breakdown  = EXCLUDED.in_breakdown,
		              out_breakdown = EXCLUDED.out_breakdown,
		              user_id       = EXCLUDED.user_id,
		              updated_at    = NOW()
	`, companyID, userID, date, carried, cashIn, cashOut,
		expected.Float64(), counted, difference.Float64(), strings.TrimSpace(note), string(inJSON), string(outJSON))
	if err != nil {
		return DayClosing{}, err
	}

	return DayClosing{
		Date:         date,
		Opening:      carried,
		CashIn:       cashIn,
		CashOut:      cashOut,
		Expected:     expected.Float64(),
		Counted:      counted,
		Difference:   difference.Float64(),
		Note:         strings.TrimSpace(note),
		Closed:       true,
		InBreakdown:  inLines,
		OutBreakdown: outLines,
	}, nil
}

// RecentClosings is the last few days, for a screen that shows whether the till has been
// running short.
//
// These come from what each closing stored, not from the figures as they stand now: the
// point of the list is what was found on the day.
func (s *LedgerService) RecentClosings(companyID int64, limit int) ([]DayClosing, error) {
	if limit <= 0 || limit > 90 {
		limit = 30
	}

	rows, err := s.db.Query(`
		SELECT TO_CHAR(closing_date, 'YYYY-MM-DD'), opening_cash, cash_in, cash_out,
		       expected_cash, counted_cash, difference, COALESCE(note, '')
		FROM day_closings
		WHERE company_id = $1
		ORDER BY closing_date DESC
		LIMIT $2
	`, companyID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []DayClosing{}
	for rows.Next() {
		var c DayClosing
		if err := rows.Scan(
			&c.Date, &c.Opening, &c.CashIn, &c.CashOut,
			&c.Expected, &c.Counted, &c.Difference, &c.Note,
		); err != nil {
			return nil, err
		}
		c.Closed = true
		out = append(out, c)
	}
	return out, rows.Err()
}
