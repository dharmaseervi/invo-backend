package services

import (
	"context"
	"database/sql"
	"invo-server/internal/models"

	"github.com/gin-gonic/gin"
)

func GetCompanyBanks(db *sql.DB, companyID int, c *gin.Context) ([]models.CompanyBank, error) {
	rows, err := db.QueryContext(c.Request.Context(),
		`
		SELECT id, company_id, account_holder_name, bank_name,
		       account_number, ifsc_code, branch, upi_id, is_default,
		       created_at, updated_at
		FROM company_bank_accounts
		WHERE company_id = $1
		ORDER BY is_default DESC, id DESC
	`, companyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	banks := []models.CompanyBank{}

	for rows.Next() {
		var b models.CompanyBank
		err := rows.Scan(
			&b.ID, &b.CompanyID, &b.AccountHolderName, &b.BankName,
			&b.AccountNumber, &b.IFSCCode, &b.Branch, &b.UPI,
			&b.IsDefault, &b.CreatedAt, &b.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		banks = append(banks, b)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	// Account numbers and IFSC codes must not reach application logs.
	return banks, nil
}

func CreateCompanyBank(ctx context.Context, db *sql.DB, b *models.CompanyBank) error {

	if b.IsDefault {
		_, _ = db.ExecContext(ctx,
			`UPDATE company_bank_accounts SET is_default = false WHERE company_id=$1`, b.CompanyID)
	}

	return db.QueryRowContext(ctx,
		`
		INSERT INTO company_bank_accounts
		(company_id, account_holder_name, bank_name, account_number, ifsc_code, branch, upi_id, is_default)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		RETURNING id
	`,
		b.CompanyID, b.AccountHolderName, b.BankName, b.AccountNumber,
		b.IFSCCode, b.Branch, b.UPI, b.IsDefault,
	).Scan(&b.ID)
}
func UpdateCompanyBank(ctx context.Context, db *sql.DB, b *models.CompanyBank) error {

	if b.IsDefault {
		// The company is read from the stored row, never from the request body: the
		// caller is only authorised for this bank account, and trusting a body-supplied
		// company_id let them clear the default bank of any company they named.
		_, _ = db.ExecContext(ctx,
			`
			UPDATE company_bank_accounts
			SET is_default = false
			WHERE company_id = (SELECT company_id FROM company_bank_accounts WHERE id = $1)
			  AND id <> $1
		`, b.ID)
	}

	_, err := db.ExecContext(ctx,
		`
		UPDATE company_bank_accounts
		SET account_holder_name=$1,
		    bank_name=$2,
		    account_number=$3,
		    ifsc_code=$4,
		    branch=$5,
		    upi_id=$6,
		    is_default=$7,
		    updated_at=NOW()
		WHERE id=$8
	`,
		b.AccountHolderName, b.BankName, b.AccountNumber,
		b.IFSCCode, b.Branch, b.UPI, b.IsDefault, b.ID,
	)

	return err
}

// DeleteCompanyBank removes one bank account.
//
// If it was the company's default, the oldest remaining account becomes the new one.
// Leaving a company with accounts but no default means an invoice printing bank
// details has nothing to pick, and the shop would have to know to go and set one.
func DeleteCompanyBank(ctx context.Context, db *sql.DB, id int) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	// Read what is being removed before removing it, so the company and whether it was
	// the default are known afterwards. From the stored row, never from the caller.
	var companyID int
	var wasDefault bool
	if err := tx.QueryRowContext(ctx,
		`
		SELECT company_id, COALESCE(is_default, false)
		FROM company_bank_accounts WHERE id = $1
	`, id).Scan(&companyID, &wasDefault); err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM company_bank_accounts WHERE id = $1`, id); err != nil {
		return err
	}

	if wasDefault {
		_, err := tx.ExecContext(ctx,
			`
			UPDATE company_bank_accounts
			SET is_default = true, updated_at = NOW()
			WHERE id = (
				SELECT id FROM company_bank_accounts
				WHERE company_id = $1
				ORDER BY id
				LIMIT 1
			)
		`, companyID)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}
