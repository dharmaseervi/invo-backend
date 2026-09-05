package services

import (
	"database/sql"
	"fmt"
	"log"
	"time"
)

// CheckOverdueInvoices runs on a timer (see StartOverdueChecker) and pushes a one-time
// "invoice overdue" notification to the owner of each invoice that has just crossed its
// due date, still has money outstanding, and hasn't been flagged before. It never
// re-notifies for the same invoice — overdue_notified is a one-way flag.
func CheckOverdueInvoices(db *sql.DB, pushService *PushService) {
	rows, err := db.Query(`
		SELECT i.id, i.user_id, i.invoice_number, i.remaining_amount, c.name
		FROM invoices i
		JOIN clients c ON c.id = i.client_id
		WHERE i.status IN ('issued', 'partial')
		  AND i.due_date < CURRENT_DATE
		  AND i.remaining_amount > 0
		  AND i.overdue_notified = false
	`)
	if err != nil {
		log.Println("overdue check: query failed:", err)
		return
	}
	defer rows.Close()

	type overdueInvoice struct {
		id              int
		userID          int
		invoiceNumber   string
		remainingAmount float64
		clientName      string
	}

	var invoices []overdueInvoice
	for rows.Next() {
		var inv overdueInvoice
		if err := rows.Scan(&inv.id, &inv.userID, &inv.invoiceNumber, &inv.remainingAmount, &inv.clientName); err == nil {
			invoices = append(invoices, inv)
		}
	}

	for _, inv := range invoices {
		pushService.SendToUser(
			inv.userID,
			"Invoice overdue",
			fmt.Sprintf("%s (%s) — ₹%.2f is now overdue", inv.invoiceNumber, inv.clientName, inv.remainingAmount),
		)
		if _, err := db.Exec(`UPDATE invoices SET overdue_notified = true WHERE id = $1`, inv.id); err != nil {
			log.Println("overdue check: failed to flag invoice", inv.id, ":", err)
		}
	}

	if len(invoices) > 0 {
		log.Printf("overdue check: notified for %d invoice(s)", len(invoices))
	}
}

// StartOverdueChecker runs CheckOverdueInvoices immediately, then on a repeating timer,
// for as long as the server process is alive — no external cron needed.
func StartOverdueChecker(db *sql.DB, pushService *PushService, interval time.Duration) {
	go func() {
		CheckOverdueInvoices(db, pushService)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			CheckOverdueInvoices(db, pushService)
		}
	}()
}
