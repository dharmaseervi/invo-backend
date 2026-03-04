package services

import (
	"context"
	"database/sql"

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

	newBalance := lastBalance + debit - credit

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
			id, company_id, client_id,
			source_type, source_id,
			debit, credit, balance,
			COALESCE(description, ''),
			created_at
		FROM ledger_entries
		WHERE company_id = $1 AND client_id = $2
		ORDER BY created_at ASC
	`, companyID, clientID)

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []models.LedgerEntry

	for rows.Next() {
		var e models.LedgerEntry
		err := rows.Scan(
			&e.ID,
			&e.CompanyID,
			&e.ClientID,
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
            id,
            company_id,
            client_id,
            source_type,
            source_id,
            debit,
            credit,
            balance,
            description,
            created_at
        FROM ledger_entries
        WHERE company_id = $1
        ORDER BY created_at ASC
    `, companyID)

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []models.LedgerEntry

	for rows.Next() {
		var e models.LedgerEntry
		err := rows.Scan(
			&e.ID,
			&e.CompanyID,
			&e.ClientID,
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
