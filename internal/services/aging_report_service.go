package services

import (
	"database/sql"
	"sort"
	"time"

	"invo-server/internal/models"
)

// GenerateAgingReport buckets every unpaid/partially-paid invoice for a company by how many
// days past its due date it is — the standard "who owes me money and for how long" report.
func GenerateAgingReport(db *sql.DB, companyID int64) (*models.AgingReportResponse, error) {

	rows, err := db.Query(`
		SELECT i.client_id, c.name, i.due_date, i.remaining_amount
		FROM invoices i
		JOIN clients c ON c.id = i.client_id
		WHERE i.company_id = $1
		  AND i.remaining_amount > 0
		  AND i.status NOT IN ('draft', 'cancelled')
	`, companyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	today := time.Now().Truncate(24 * time.Hour)

	clientOrder := []int{}
	clientByID := map[int]*models.ClientAgingRow{}
	totals := models.AgingBucket{}
	var grandTotal float64

	for rows.Next() {
		var (
			clientID    int
			clientName  string
			dueDate     time.Time
			outstanding float64
		)
		if err := rows.Scan(&clientID, &clientName, &dueDate, &outstanding); err != nil {
			return nil, err
		}

		daysOverdue := int(today.Sub(dueDate.Truncate(24 * time.Hour)).Hours() / 24)

		row, exists := clientByID[clientID]
		if !exists {
			row = &models.ClientAgingRow{ClientID: clientID, ClientName: clientName}
			clientByID[clientID] = row
			clientOrder = append(clientOrder, clientID)
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

	clients := make([]models.ClientAgingRow, 0, len(clientOrder))
	for _, id := range clientOrder {
		clients = append(clients, *clientByID[id])
	}
	// Worst-affected clients (highest outstanding) first.
	sort.Slice(clients, func(i, j int) bool { return clients[i].Total > clients[j].Total })

	return &models.AgingReportResponse{
		AsOf:       today.Format("2006-01-02"),
		GrandTotal: grandTotal,
		Totals:     totals,
		Clients:    clients,
	}, nil
}
