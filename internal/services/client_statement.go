package services

import (
	"context"
	"database/sql"
	"time"

	"invo-server/internal/models"
)

// A customer's statement of account, for a period.
//
// The app already shows a customer's ledger on screen. This is the thing a shop sends
// *to* the customer when they ring up and say they have paid everything — one page,
// with what was owed at the start, everything that happened, and what is owed now. It
// is the document that settles the argument.

// ClientStatement is one customer's account over a period.
type ClientStatement struct {
	CompanyName    string
	CompanyGSTIN   string
	CompanyPhone   string
	CompanyAddress string

	ClientName    string
	ClientPhone   string
	ClientAddress string

	From time.Time
	To   time.Time

	// What they owed before the period began.
	Opening float64
	Entries []models.LedgerEntry
	// Totals of the period alone.
	Billed float64
	Paid   float64
	// What they owe at the end of it.
	Closing float64
}

// ClientStatementFor builds a customer's statement between two dates, inclusive.
//
// The opening balance is read from the running balance of the last entry before the
// period rather than being added up here. That column is what the ledger itself has
// always said the customer owed at that moment, and a statement that recomputed it
// could disagree with the app's own screens — which is exactly the argument a statement
// is sent to end.
func (s *LedgerService) ClientStatementFor(
	ctx context.Context,
	companyID, clientID int64,
	from, to time.Time,
) (ClientStatement, error) {
	out := ClientStatement{From: from, To: to}

	// The two parties, and the check that this customer is this company's.
	err := s.db.QueryRowContext(ctx, `
		SELECT
			COALESCE(co.name, ''), COALESCE(co.gst, ''), COALESCE(co.phone, ''),
			TRIM(BOTH ', ' FROM CONCAT_WS(', ',
				NULLIF(co.address, ''), NULLIF(co.city, ''),
				NULLIF(co.state, ''), NULLIF(co.pincode, ''))),
			cl.name, COALESCE(cl.phone, ''),
			TRIM(BOTH ', ' FROM CONCAT_WS(', ',
				NULLIF(cl.address, ''), NULLIF(cl.city, ''),
				NULLIF(cl.state, ''), NULLIF(cl.pincode, '')))
		FROM clients cl
		JOIN companies co ON co.id = cl.company_id
		WHERE cl.id = $1 AND cl.company_id = $2
	`, clientID, companyID).Scan(
		&out.CompanyName, &out.CompanyGSTIN, &out.CompanyPhone, &out.CompanyAddress,
		&out.ClientName, &out.ClientPhone, &out.ClientAddress,
	)
	if err == sql.ErrNoRows {
		return out, LedgerInputError{"That customer isn't one of this company's."}
	}
	if err != nil {
		return out, err
	}

	// Where the account stood the moment before the period opened.
	err = s.db.QueryRowContext(ctx, `
		SELECT COALESCE((
			SELECT balance FROM ledger_entries
			WHERE company_id = $1 AND client_id = $2 AND created_at < $3
			ORDER BY created_at DESC, id DESC
			LIMIT 1
		), 0)
	`, companyID, clientID, from).Scan(&out.Opening)
	if err != nil {
		return out, err
	}

	// `to` is a date, and a date means the whole of that day — an entry at half past
	// four in the afternoon on the closing date belongs in the statement.
	end := to.AddDate(0, 0, 1)

	rows, err := s.db.QueryContext(ctx, `
		SELECT le.id, le.company_id, le.client_id, c.name,
		       le.source_type, le.source_id, le.debit, le.credit, le.balance,
		       COALESCE(le.description, ''), le.created_at
		FROM ledger_entries le
		JOIN clients c ON c.id = le.client_id
		WHERE le.company_id = $1 AND le.client_id = $2
		  AND le.created_at >= $3 AND le.created_at < $4
		ORDER BY le.created_at ASC, le.id ASC
	`, companyID, clientID, from, end)
	if err != nil {
		return out, err
	}
	defer rows.Close()

	out.Entries = []models.LedgerEntry{}
	for rows.Next() {
		var e models.LedgerEntry
		if err := rows.Scan(
			&e.ID, &e.CompanyID, &e.ClientID, &e.ClientName,
			&e.SourceType, &e.SourceID, &e.Debit, &e.Credit, &e.Balance,
			&e.Description, &e.CreatedAt,
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

	// The closing figure is the last line's running balance, not opening plus the
	// period's movement. They agree, and when they ever do not it is the ledger that
	// is right and this sum that is wrong.
	if n := len(out.Entries); n > 0 {
		out.Closing = out.Entries[n-1].Balance
	} else {
		out.Closing = out.Opening
	}

	return out, nil
}

// LedgerInputError is a problem the person can fix, as opposed to a failure.
type LedgerInputError struct{ Msg string }

func (e LedgerInputError) Error() string { return e.Msg }
