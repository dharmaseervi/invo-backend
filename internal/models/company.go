package models

type Company struct {
	ID      int    `json:"id"`
	UserID  int    `json:"user_id"`
	Name    string `json:"name"`
	Address string `json:"address"`
	Phone   string `json:"phone"`
	Gst     string `json:"gst"`
	City    string `json:"city"`
	State   string `json:"state"`
	Pincode string `json:"pincode"`
}
