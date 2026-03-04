package services

import (
	"database/sql"
	"fmt"
	"invo-server/internal/models"

	"github.com/gin-gonic/gin"
)

func GetCompanyBanks(db *sql.DB, companyID int, c *gin.Context) ([]models.CompanyBank, error) {
	rows, err := db.Query(`
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

	var banks []models.CompanyBank

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

	fmt.Println(banks)
	return banks, nil
}

func CreateCompanyBank(db *sql.DB, b *models.CompanyBank) error {

	if b.IsDefault {
		_, _ = db.Exec(`UPDATE company_bank_accounts SET is_default = false WHERE company_id=$1`, b.CompanyID)
	}

	return db.QueryRow(`
		INSERT INTO company_bank_accounts
		(company_id, account_holder_name, bank_name, account_number, ifsc_code, branch, upi_id, is_default)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		RETURNING id
	`,
		b.CompanyID, b.AccountHolderName, b.BankName, b.AccountNumber,
		b.IFSCCode, b.Branch, b.UPI, b.IsDefault,
	).Scan(&b.ID)
}
func UpdateCompanyBank(db *sql.DB, b *models.CompanyBank) error {

	if b.IsDefault {
		_, _ = db.Exec(`UPDATE company_bank_accounts SET is_default=false WHERE company_id=$1`, b.CompanyID)
	}

	_, err := db.Exec(`
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
