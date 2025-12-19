package models

type Expensess struct {
	ID          int     `json:"id"`
	UserID      int     `json:"user_id"`
	CompanyID   int     `json:"company_id"`
	Name        string  `json:"name"`
	Amount      float64 `json:"amount"`
	Description string  `json:"description"`
	Date        string  `json:"date"`
	CreatedAt   string  `json:"created_at"`
	UpdatedAt   string  `json:"updated_at"`
}
