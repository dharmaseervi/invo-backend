package models

type Category struct {
	ID        int    `json:"id"`
	UserID    int    `json:"user_id"`
	CompanyID int    `json:"company_id"`
	Name      string `json:"name"`
}
