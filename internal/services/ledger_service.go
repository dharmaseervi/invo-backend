package services

import (
	"context"
	"database/sql"
	"strconv"

	"invo-server/internal/models"
	"invo-server/internal/money"
)

type LedgerService struct {
	db *sql.DB
}

func NewLedgerService(db *sql.DB) *LedgerService {
	return &LedgerService{db: db}
}

// Get last balance
func (s *LedgerService) getLastBalanceTx(
	tx *sql.Tx,
	companyID, clientID int64,
) (float64, error) {

	var balance float64

	// Lock the client row, not the newest ledger row.
	//
	// Entries for one client have to be serialised: two payments recorded at once both
	// read the same prior balance and both write "balance - amount", so one of them
	// vanishes from the running total even though both rows exist. Locking the newest
	// entry looked like it did that, but it locks nothing when the client has no entries
	// yet — a first invoice and a first payment arriving together — and concurrent
	// writers each insert their own row anyway, so there is no shared row to contend on.
	// The client row always exists and is the same row for every writer, so it
	// serialises them all.
	if _, err := tx.Exec(`
		SELECT 1 FROM clients WHERE id = $1 AND company_id = $2 FOR UPDATE
	`, clientID, companyID); err != nil {
		return 0, err
	}

	err := tx.QueryRow(`
		SELECT balance
		FROM ledger_entries
		WHERE company_id = $1 AND client_id = $2
		ORDER BY id DESC
		LIMIT 1
	`, companyID, clientID).Scan(&balance)

	if err == sql.ErrNoRows {
		return 0, nil
	}

	return balance, err
}

func (s *LedgerService) AddEntryTx(
	tx *sql.Tx,
	companyID int64,
	clientID int64,
	sourceType string,
	sourceID int64,
	debit float64,
	credit float64,
	description string,
) error {

	lastBalance, err := s.getLastBalanceTx(tx, companyID, clientID)
	if err != nil {
		return err
	}

	// Decimal: this balance is carried forward into every later entry, so a fraction of
	// a paisa of float drift here compounds down the whole statement.
	newBalance := money.FromFloat(lastBalance).
		Add(money.FromFloat(debit)).
		Sub(money.FromFloat(credit)).
		Round().
		Float64()

	_, err = tx.Exec(`
		INSERT INTO ledger_entries (
			company_id,
			client_id,
			source_type,
			source_id,
			debit,
			credit,
			balance,
			description
		)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
	`,
		companyID,
		clientID,
		sourceType,
		sourceID,
		debit,
		credit,
		newBalance,
		description,
	)

	return err
}

// Fetch full ledger
// GetClientLedger returns a customer's statement, oldest first, because a running
// balance only makes sense read forwards.
//
// limit of 0 means the whole statement, which is what the apps in the store ask for. A
// customer who has been buying for years has thousands of lines, and every one of them
// was fetched, sent and rendered to show the last few — so a caller that pages takes
// the most recent `limit` entries and still receives them oldest first.
func (s *LedgerService) GetClientLedger(
	ctx context.Context,
	companyID, clientID int64,
	limit, offset int,
) ([]models.LedgerEntry, error) {

	query := `
		SELECT
    le.id,
    le.company_id,
    le.client_id,
    c.name AS client_name,
    le.source_type,
    le.source_id,
    le.debit,
    le.credit,
    le.balance,
    COALESCE(le.description, ''),
    le.created_at
FROM ledger_entries le
JOIN clients c ON c.id = le.client_id
WHERE le.company_id = $1 AND le.client_id = $2
	`
	args := []interface{}{companyID, clientID}
	if limit > 0 {
		// The newest page, then turned back the right way round: paging walks backwards
		// through the statement while each page still reads forwards.
		query = `SELECT * FROM (` + query + `
			ORDER BY le.created_at DESC, le.id DESC LIMIT $3 OFFSET $4
		) page ORDER BY created_at ASC, id ASC`
		args = append(args, limit, offset)
	} else {
		query += " ORDER BY le.created_at ASC, le.id ASC"
	}

	rows, err := s.db.QueryContext(ctx, query, args...)

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	entries := []models.LedgerEntry{}

	for rows.Next() {
		var e models.LedgerEntry
		err := rows.Scan(
			&e.ID,
			&e.CompanyID,
			&e.ClientID,
			&e.ClientName,
			&e.SourceType,
			&e.SourceID,
			&e.Debit,
			&e.Credit,
			&e.Balance,
			&e.Description,
			&e.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}

	return entries, nil
}

// GetCompanyLedger is the whole company's ledger, oldest first, for the same reason as
// a customer's: a running balance reads forwards.
//
// limit of 0 means everything, which is what the apps in the store ask for. A company
// ledger grows with every invoice and payment the business has ever made, so a caller
// that pages takes the newest page and still receives it oldest-first.
func (s *LedgerService) GetCompanyLedger(
	ctx context.Context,
	companyID int64,
	limit, offset int,
) ([]models.LedgerEntry, error) {

	query := `
	SELECT
    le.id,
    le.company_id,
    le.client_id,
    c.name AS client_name,   -- ✅ GET FROM CLIENTS
    le.source_type,
    le.source_id,
    le.debit,
    le.credit,
    le.balance,
    le.description,
    le.created_at
FROM ledger_entries le
JOIN clients c ON c.id = le.client_id   -- ✅ THIS IS KEY
WHERE le.company_id = $1
    `
	args := []interface{}{companyID}
	if limit > 0 {
		query = `SELECT * FROM (` + query + `
			ORDER BY le.created_at DESC, le.id DESC LIMIT $2 OFFSET $3
		) page ORDER BY created_at ASC, id ASC`
		args = append(args, limit, offset)
	} else {
		query += " ORDER BY le.created_at ASC, le.id ASC"
	}

	rows, err := s.db.QueryContext(ctx, query, args...)

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	entries := []models.LedgerEntry{}

	for rows.Next() {
		var e models.LedgerEntry
		err := rows.Scan(
			&e.ID,
			&e.CompanyID,
			&e.ClientID,
			&e.ClientName,
			&e.SourceType,
			&e.SourceID,
			&e.Debit,
			&e.Credit,
			&e.Balance,
			&e.Description,
			&e.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}

	return entries, nil
}

// ClientLedgerSummary is one customer's totals over their whole history.
//
// The balance is taken from their most recent entry rather than debit minus credit:
// the running balance is what every other screen quotes, and it is the figure the
// customer is shown on a statement.
func (s *LedgerService) ClientLedgerSummary(
	ctx context.Context,
	companyID, clientID int64,
) (models.LedgerSummary, error) {
	var out models.LedgerSummary
	out.ClientID = clientID

	err := s.db.QueryRowContext(ctx, `
		SELECT
			COALESCE(MAX(c.name), ''),
			COALESCE(SUM(le.debit), 0),
			COALESCE(SUM(le.credit), 0),
			COALESCE((
				SELECT balance FROM ledger_entries
				WHERE company_id = $1 AND client_id = $2
				ORDER BY created_at DESC, id DESC LIMIT 1
			), 0),
			COUNT(le.id),
			MAX(le.created_at)
		FROM ledger_entries le
		JOIN clients c ON c.id = le.client_id
		WHERE le.company_id = $1 AND le.client_id = $2
	`, companyID, clientID).Scan(
		&out.ClientName, &out.Debit, &out.Credit, &out.Balance, &out.Entries, &out.LastEntryAt,
	)
	return out, err
}

// CompanyLedgerSummaries is one row per customer who has any ledger history, which is
// what a ledger list screen actually shows.
//
// It replaces fetching every entry the company has ever written and grouping them in
// the app: that meant downloading a whole business's history to draw a list of names
// and balances, and a paged version of it would have given each customer the balance
// they happened to have part-way through.
func (s *LedgerService) CompanyLedgerSummaries(
	ctx context.Context,
	companyID int64,
	search string,
	limit, offset int,
) ([]models.LedgerSummary, error) {
	query := `
		SELECT
			le.client_id,
			MAX(c.name),
			COALESCE(SUM(le.debit), 0),
			COALESCE(SUM(le.credit), 0),
			COALESCE((
				SELECT balance FROM ledger_entries inner_le
				WHERE inner_le.company_id = le.company_id AND inner_le.client_id = le.client_id
				ORDER BY inner_le.created_at DESC, inner_le.id DESC LIMIT 1
			), 0),
			COUNT(le.id),
			MAX(le.created_at)
		FROM ledger_entries le
		JOIN clients c ON c.id = le.client_id
		WHERE le.company_id = $1
	`
	args := []interface{}{companyID}
	if search != "" {
		query += " AND c.name ILIKE $2"
		args = append(args, "%"+search+"%")
	}
	// Busiest first, then by id so the order is fixed between pages.
	query += `
		GROUP BY le.client_id, le.company_id
		ORDER BY MAX(le.created_at) DESC, le.client_id DESC
	`
	if limit > 0 {
		query += " LIMIT $" + strconv.Itoa(len(args)+1) + " OFFSET $" + strconv.Itoa(len(args)+2)
		args = append(args, limit, offset)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []models.LedgerSummary{}
	for rows.Next() {
		var r models.LedgerSummary
		if err := rows.Scan(
			&r.ClientID, &r.ClientName, &r.Debit, &r.Credit, &r.Balance, &r.Entries, &r.LastEntryAt,
		); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
