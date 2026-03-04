package models

import "time"

type CompanyBank struct {
	ID                int       `json:"id"`
	CompanyID         int       `json:"company_id"`
	AccountHolderName string    `json:"account_holder_name"`
	BankName          string    `json:"bank_name"`
	AccountNumber     string    `json:"account_number"`
	IFSCCode          string    `json:"ifsc_code"`
	Branch            string    `json:"branch"`
	UPI               string    `json:"upi_id"`
	IsDefault         bool      `json:"is_default"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}
