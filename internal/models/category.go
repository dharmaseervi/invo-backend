package models

type Category struct {
	ID             int      `json:"id"`
	UserID         int      `json:"user_id"`
	CompanyID      int      `json:"company_id"`
	Name           string   `json:"name"`
	DefaultHSNCode *string  `json:"default_hsn_code,omitempty"`
	DefaultTaxRate *float64 `json:"default_tax_rate,omitempty"`
}
