package services

import (
	"context"
	"database/sql"
	"invo-server/internal/money"

	"invo-server/internal/models"
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
func (s *LedgerService) GetClientLedger(
	ctx context.Context,
	companyID, clientID int64,
) ([]models.LedgerEntry, error) {

	rows, err := s.db.QueryContext(ctx, `
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
ORDER BY le.created_at ASC
	`, companyID, clientID)

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

func (s *LedgerService) GetCompanyLedger(
	ctx context.Context,
	companyID int64,
) ([]models.LedgerEntry, error) {

	rows, err := s.db.QueryContext(ctx, `
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
ORDER BY le.created_at ASC
    `, companyID)

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
