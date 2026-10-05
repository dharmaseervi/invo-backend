package services

import (
	"database/sql"
	"strings"

	"invo-server/internal/money"
)

// Counting the cash drawer at the end of the day.
//
// The app knew what the day's cash billing came to. Whether that much cash was actually
// in the drawer was a separate question it could not ask — so a short till was found
// weeks later, if at all, with no way to tell which day it went missing.

// DayClosing is one day's cash, expected against counted.
type DayClosing struct {
	Date     string  `json:"date"`
	Expected float64 `json:"expected_cash"`
	Counted  float64 `json:"counted_cash"`
	// Counted less expected: negative is short, positive is over.
	Difference float64 `json:"difference"`
	Note       string  `json:"note"`
	Closed     bool    `json:"closed"`
	// How the expected figure was arrived at, so a difference can be chased rather
	// than just stared at.
	PaymentCount int `json:"payment_count"`
}

// ExpectedCashFor is what the drawer should hold for a date: every cash payment taken
// that day, less any that were reversed.
//
// Reversals are left out because the money went back out again — counting them would
// have the till look short by exactly the amount that was handed back.
func (s *LedgerService) ExpectedCashFor(companyID int64, date string) (float64, int, error) {
	var total sql.NullFloat64
	var count int

	err := s.db.QueryRow(`
		SELECT COALESCE(SUM(amount), 0), COUNT(*)
		FROM payments
		WHERE company_id = $1
		  AND payment_date = $2::date
		  AND status <> 'reversed'
		  AND lower(COALESCE(payment_method, '')) IN ('cash', 'cash payment')
	`, companyID, date).Scan(&total, &count)
	if err != nil {
		return 0, 0, err
	}
	return total.Float64, count, nil
}

// ClosingFor reads a day: what is expected, and what was counted if the day has been
// closed already.
func (s *LedgerService) ClosingFor(companyID int64, date string) (DayClosing, error) {
	expected, count, err := s.ExpectedCashFor(companyID, date)
	if err != nil {
		return DayClosing{}, err
	}

	out := DayClosing{
		Date:         date,
		Expected:     expected,
		PaymentCount: count,
	}

	var note sql.NullString
	err = s.db.QueryRow(`
		SELECT counted_cash, difference, COALESCE(note, '')
		FROM day_closings
		WHERE company_id = $1 AND closing_date = $2::date
	`, companyID, date).Scan(&out.Counted, &out.Difference, &note)

	switch {
	case err == sql.ErrNoRows:
		// Not closed yet. The expected figure still stands, so the screen can show
		// what should be there before anybody counts.
		return out, nil
	case err != nil:
		return out, err
	}

	out.Note = note.String
	out.Closed = true
	return out, nil
}

// Close records what was counted out of the drawer.
//
// The expected figure is stored alongside, not just the difference: a payment corrected
// next week would otherwise silently change what last Tuesday's closing appeared to
// have found. A closing is a record of what was true when somebody counted.
//
// Closing the same day twice replaces the figure rather than adding a second one —
// recounting is correcting the first count, not a separate event.
func (s *LedgerService) Close(
	companyID, userID int64,
	date string,
	counted float64,
	note string,
) (DayClosing, error) {
	if counted < 0 {
		return DayClosing{}, LedgerInputError{"A cash count cannot be negative."}
	}

	expected, count, err := s.ExpectedCashFor(companyID, date)
	if err != nil {
		return DayClosing{}, err
	}

	difference := money.FromFloat(counted).Sub(money.FromFloat(expected)).Round()

	_, err = s.db.Exec(`
		INSERT INTO day_closings
			(company_id, user_id, closing_date, expected_cash, counted_cash, difference, note)
		VALUES ($1, $2, $3::date, $4, $5, $6, $7)
		ON CONFLICT (company_id, closing_date)
		DO UPDATE SET expected_cash = EXCLUDED.expected_cash,
		              counted_cash  = EXCLUDED.counted_cash,
		              difference    = EXCLUDED.difference,
		              note          = EXCLUDED.note,
		              user_id       = EXCLUDED.user_id,
		              updated_at    = NOW()
	`, companyID, userID, date, expected, counted, difference.Float64(), strings.TrimSpace(note))
	if err != nil {
		return DayClosing{}, err
	}

	return DayClosing{
		Date:         date,
		Expected:     expected,
		Counted:      counted,
		Difference:   difference.Float64(),
		Note:         strings.TrimSpace(note),
		Closed:       true,
		PaymentCount: count,
	}, nil
}

// RecentClosings is the last few days, for a screen that shows whether the till has been
// running short.
func (s *LedgerService) RecentClosings(companyID int64, limit int) ([]DayClosing, error) {
	if limit <= 0 || limit > 90 {
		limit = 30
	}

	// The payment count is worked out per day rather than left at zero: a list row
	// claiming a day had no cash payments, when the figures beside it plainly came
	// from some, is worse than not saying at all.
	rows, err := s.db.Query(`
		SELECT TO_CHAR(d.closing_date, 'YYYY-MM-DD'), d.expected_cash, d.counted_cash,
		       d.difference, COALESCE(d.note, ''),
		       (SELECT COUNT(*) FROM payments p
		        WHERE p.company_id = d.company_id
		          AND p.payment_date = d.closing_date
		          AND p.status <> 'reversed'
		          AND lower(COALESCE(p.payment_method, '')) IN ('cash', 'cash payment'))
		FROM day_closings d
		WHERE d.company_id = $1
		ORDER BY d.closing_date DESC
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
			&c.Date, &c.Expected, &c.Counted, &c.Difference, &c.Note, &c.PaymentCount,
		); err != nil {
			return nil, err
		}
		c.Closed = true
		out = append(out, c)
	}
	return out, rows.Err()
}
