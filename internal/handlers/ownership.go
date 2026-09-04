package handlers

import "database/sql"

// companyBelongsToUser reports whether companyID is owned by userID.
// Used to prevent IDOR — callers must reject the request (403/404) when this returns false.
func companyBelongsToUser(db *sql.DB, companyID int64, userID int) (bool, error) {
	var exists bool
	err := db.QueryRow(
		`SELECT EXISTS (SELECT 1 FROM companies WHERE id = $1 AND user_id = $2)`,
		companyID, userID,
	).Scan(&exists)
	return exists, err
}

// bankBelongsToUser reports whether bankID's company is owned by userID.
func bankBelongsToUser(db *sql.DB, bankID int, userID int) (bool, error) {
	var exists bool
	err := db.QueryRow(`
		SELECT EXISTS (
			SELECT 1 FROM company_bank_accounts b
			JOIN companies c ON c.id = b.company_id
			WHERE b.id = $1 AND c.user_id = $2
		)
	`, bankID, userID).Scan(&exists)
	return exists, err
}

// invoiceBelongsToUser reports whether invoiceID's company is owned by userID.
func invoiceBelongsToUser(db *sql.DB, invoiceID int, userID int) (bool, error) {
	var exists bool
	err := db.QueryRow(`
		SELECT EXISTS (
			SELECT 1 FROM invoices i
			JOIN companies c ON c.id = i.company_id
			WHERE i.id = $1 AND c.user_id = $2
		)
	`, invoiceID, userID).Scan(&exists)
	return exists, err
}
