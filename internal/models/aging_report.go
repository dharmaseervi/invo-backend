package models

type AgingBucket struct {
	Current    float64 `json:"current"`     // not yet due
	Days1to30  float64 `json:"days_1_30"`
	Days31to60 float64 `json:"days_31_60"`
	Days61to90 float64 `json:"days_61_90"`
	Days90Plus float64 `json:"days_90_plus"`
}

type ClientAgingRow struct {
	ClientID   int         `json:"client_id"`
	ClientName string      `json:"client_name"`
	Total      float64     `json:"total"`
	Buckets    AgingBucket `json:"buckets"`
}

type AgingReportResponse struct {
	AsOf       string           `json:"as_of"`
	GrandTotal float64          `json:"grand_total"`
	Totals     AgingBucket      `json:"totals"`
	Clients    []ClientAgingRow `json:"clients"`
}
