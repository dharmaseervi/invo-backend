package services

import "database/sql"

// dbExecutor is satisfied by both *sql.DB and *sql.Tx, so movement logging works
// the same whether it happens standalone (item edit, restock) or inside a
// transaction (invoice issue, which deducts stock for several items at once).
type dbExecutor interface {
	Exec(query string, args ...any) (sql.Result, error)
}

// LogStockMovement records one entry in an item's stock audit trail.
func LogStockMovement(
	db dbExecutor,
	itemID, companyID, userID int,
	movementType string,
	quantityChange, previousQuantity, newQuantity int,
	reference, note *string,
) error {
	_, err := db.Exec(`
		INSERT INTO stock_movements
			(item_id, company_id, user_id, movement_type, quantity_change, previous_quantity, new_quantity, reference, note)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, itemID, companyID, userID, movementType, quantityChange, previousQuantity, newQuantity, reference, note)
	return err
}
