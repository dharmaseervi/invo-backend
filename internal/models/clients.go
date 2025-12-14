package models

import (
	"time"
)

type Client struct {
	ID        int       `json:"id"`
	UserID    int       `json:"user_id"`
	CompanyID int       `json:"company_id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	Phone     string    `json:"phone"`
	Address   string    `json:"address"`
	City      string    `json:"city"`
	State     string    `json:"state"`
	Pincode   string    `json:"pincode"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
