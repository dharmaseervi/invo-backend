package services

import (
	"context"
	"database/sql"
	"sort"
	"time"

	"invo-server/internal/models"
)

// SupplierAgingRow mirrors ClientAgingRow but for purchases owed to suppliers.
type SupplierAgingRow struct {
	SupplierID   int                `json:"supplier_id"`
	SupplierName string             `json:"supplier_name"`
	Total        float64            `json:"total"`
	Buckets      models.AgingBucket `json:"buckets"`
}

type SupplierAgingReport struct {
	AsOf       string             `json:"as_of"`
	GrandTotal float64            `json:"grand_total"`
	Totals     models.AgingBucket `json:"totals"`
	Suppliers  []SupplierAgingRow `json:"suppliers"`
}

// GenerateSupplierAgingReport buckets unpaid/partially-paid purchase bills by how many
// days past their due date they are — the "what do I owe and for how long" report.
func GenerateSupplierAgingReport(ctx context.Context, db *sql.DB, companyID int64) (*SupplierAgingReport, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT pb.supplier_id, s.name, pb.due_date, pb.remaining_amount
		 FROM purchase_bills pb
		 JOIN suppliers s ON s.id = pb.supplier_id
		 WHERE pb.company_id = $1
		   AND pb.remaining_amount > 0
		   AND pb.status NOT IN ('draft', 'cancelled')`,
		companyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	today := time.Now().Truncate(24 * time.Hour)

	supplierOrder := []int{}
	supplierByID := map[int]*SupplierAgingRow{}
	totals := models.AgingBucket{}
	var grandTotal float64

	for rows.Next() {
		var (
			supplierID   int
			supplierName string
			dueDate      time.Time
			outstanding  float64
		)
		if err := rows.Scan(&supplierID, &supplierName, &dueDate, &outstanding); err != nil {
			return nil, err
		}

		daysOverdue := int(today.Sub(dueDate.Truncate(24*time.Hour)).Hours() / 24)

		row, exists := supplierByID[supplierID]
		if !exists {
			row = &SupplierAgingRow{SupplierID: supplierID, SupplierName: supplierName}
			supplierByID[supplierID] = row
			supplierOrder = append(supplierOrder, supplierID)
		}

		switch {
		case daysOverdue <= 0:
			row.Buckets.Current += outstanding
			totals.Current += outstanding
		case daysOverdue <= 30:
			row.Buckets.Days1to30 += outstanding
			totals.Days1to30 += outstanding
		case daysOverdue <= 60:
			row.Buckets.Days31to60 += outstanding
			totals.Days31to60 += outstanding
		case daysOverdue <= 90:
			row.Buckets.Days61to90 += outstanding
			totals.Days61to90 += outstanding
		default:
			row.Buckets.Days90Plus += outstanding
			totals.Days90Plus += outstanding
		}
		row.Total += outstanding
		grandTotal += outstanding
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	suppliers := make([]SupplierAgingRow, 0, len(supplierOrder))
	for _, id := range supplierOrder {
		suppliers = append(suppliers, *supplierByID[id])
	}
	sort.Slice(suppliers, func(i, j int) bool { return suppliers[i].Total > suppliers[j].Total })

	return &SupplierAgingReport{
		AsOf:       today.Format("2006-01-02"),
		GrandTotal: grandTotal,
		Totals:     totals,
		Suppliers:  suppliers,
	}, nil
}
