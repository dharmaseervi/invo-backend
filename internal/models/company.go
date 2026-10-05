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

	// What the person asking is to this business: owner, manager or staff. Left out
	// where it was not asked for, so the shape an older app decodes is unchanged.
	Role string `json:"role,omitempty"`
}
