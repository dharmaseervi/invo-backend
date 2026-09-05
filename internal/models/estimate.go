package models

type EstimateRequestDTO struct {
	CompanyID    int                  `json:"company_id"`
	ClientID     int                  `json:"client_id"`
	EstimateDate string               `json:"estimate_date"` // YYYY-MM-DD
	ExpiryDate   *string              `json:"expiry_date"`   // YYYY-MM-DD, optional
	Discount     float64              `json:"discount"`
	Items        []InvoiceItemRequest `json:"items"`
}

type UpdateEstimateRequestDTO struct {
	ClientID     int                  `json:"client_id"`
	EstimateDate string               `json:"estimate_date"`
	ExpiryDate   *string              `json:"expiry_date"`
	Discount     float64              `json:"discount"`
	Items        []InvoiceItemRequest `json:"items"`
}

type EstimateStatusUpdateDTO struct {
	Status string `json:"status" binding:"required"` // sent | accepted | rejected
}

type CreateEstimateResponse struct {
	EstimateID     int    `json:"estimate_id"`
	EstimateNumber string `json:"estimate_number"`
	FinancialYear  string `json:"financial_year"`
}
