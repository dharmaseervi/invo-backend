package models

type Expensess struct {
	ID          int     `json:"id"`
	UserID      int     `json:"user_id"`
	CompanyID   int     `json:"company_id"`
	Name        string  `json:"name"`
	Amount      float64 `json:"amount"`
	Description string  `json:"description"`
	Date        string  `json:"date"`
	// How it was paid. Empty where nobody said, which is not the same as cash — the
	// cash drawer only counts an expense it was told came out of the till.
	PaymentMethod string `json:"payment_method"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
}
