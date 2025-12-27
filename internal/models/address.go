package models

type Address struct {
	ID          int    `json:"id,omitempty"`
	AddressType string `json:"type"` // billing | shipping

	Name  *string `json:"name,omitempty"`
	Line1 string  `json:"line1"`

	Line2      *string `json:"line2,omitempty"`
	City       *string `json:"city,omitempty"`
	State      *string `json:"state,omitempty"`
	PostalCode *string `json:"postal_code,omitempty"`
	Country    *string `json:"country,omitempty"`

	Phone     *string `json:"phone,omitempty"`
	Email     *string `json:"email,omitempty"`
	GSTNumber *string `json:"gst_number,omitempty"`
}
