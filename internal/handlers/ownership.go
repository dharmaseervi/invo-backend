package handlers

import "database/sql"

// Who is allowed to touch a business's records.
//
// These three answer the same question about different things, and every handler that
// takes an id from a request has to ask one of them. They are what stops a request
// naming somebody else's invoice from being served.
//
// "Belongs to" now means the user works in that business, not that they own it: an
// owner, a manager or a counter boy all pass here. What each of them may then *do* is
// the permission policy's question, answered before the handler runs. Keeping the two
// apart means a staff account reads a customer's name through exactly the same code
// path the owner does, with no second version of it to drift.

// companyBelongsToUser reports whether userID works in companyID.
// Used to prevent IDOR — callers must reject the request (403/404) when this returns false.
func companyBelongsToUser(db *sql.DB, companyID int64, userID int) (bool, error) {
	var exists bool
	err := db.QueryRow(`
		SELECT EXISTS (
			-- The owner's own row. Checked as well as membership rather than
			-- instead of it: if a company is ever created without its member row,
			-- this keeps the owner out of a business they own.
			SELECT 1 FROM companies WHERE id = $1 AND user_id = $2
			UNION ALL
			SELECT 1 FROM company_members WHERE company_id = $1 AND user_id = $2
		)
	`, companyID, userID).Scan(&exists)
	return exists, err
}

// bankBelongsToUser reports whether bankID's company is one userID works in.
func bankBelongsToUser(db *sql.DB, bankID int, userID int) (bool, error) {
	var exists bool
	err := db.QueryRow(`
		SELECT EXISTS (
			SELECT 1 FROM company_bank_accounts b
			JOIN companies c ON c.id = b.company_id
			LEFT JOIN company_members m ON m.company_id = c.id AND m.user_id = $2
			WHERE b.id = $1 AND (c.user_id = $2 OR m.id IS NOT NULL)
		)
	`, bankID, userID).Scan(&exists)
	return exists, err
}

// invoiceBelongsToUser reports whether invoiceID's company is one userID works in.
func invoiceBelongsToUser(db *sql.DB, invoiceID int, userID int) (bool, error) {
	var exists bool
	err := db.QueryRow(`
		SELECT EXISTS (
			SELECT 1 FROM invoices i
			JOIN companies c ON c.id = i.company_id
			LEFT JOIN company_members m ON m.company_id = c.id AND m.user_id = $2
			WHERE i.id = $1 AND (c.user_id = $2 OR m.id IS NOT NULL)
		)
	`, invoiceID, userID).Scan(&exists)
	return exists, err
}
