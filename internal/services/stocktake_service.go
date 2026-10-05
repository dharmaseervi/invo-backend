package services

import (
	"database/sql"
	"strings"
)

// Counting the floor.
//
// Stock in the app only ever moved by something the app saw: a sale, a purchase, a
// restock typed in. Everything else — a broken box, a bundle handed to a mason and never
// billed, two boxes that were never there — accumulated silently, and the figure on the
// screen drifted away from the floor with nothing to pull it back.

// StocktakeInputError is a problem the person can fix.
type StocktakeInputError struct{ Msg string }

func (e StocktakeInputError) Error() string { return e.Msg }

type StocktakeService struct {
	db *sql.DB
}

func NewStocktakeService(db *sql.DB) *StocktakeService {
	return &StocktakeService{db: db}
}

// StocktakeLine is one item as counted.
type StocktakeLine struct {
	ItemID   int64  `json:"item_id"`
	ItemName string `json:"item_name"`
	Unit     string `json:"unit"`
	// What the books said when it was counted, and what was found.
	Expected int `json:"expected"`
	Counted  int `json:"counted"`
}

// Variance is what the count found: positive means more on the floor than on the books.
func (l StocktakeLine) Variance() int { return l.Counted - l.Expected }

// Stocktake is one count of the floor.
type Stocktake struct {
	ID        int64           `json:"id"`
	Status    string          `json:"status"`
	Note      string          `json:"note"`
	StartedAt string          `json:"started_at"`
	AppliedAt string          `json:"applied_at"`
	Lines     []StocktakeLine `json:"lines"`

	// Counted so a list can say what a session found without its lines.
	ItemsCounted int `json:"items_counted"`
	ItemsOff     int `json:"items_off"`
}

// Start opens a count. One draft at a time per company: two people counting the same
// floor into two sessions would each apply their own variance, and the second would
// correct stock the first had already corrected.
func (s *StocktakeService) Start(companyID, userID int64, note string) (int64, error) {
	var existing int64
	err := s.db.QueryRow(`
		SELECT id FROM stocktakes
		WHERE company_id = $1 AND status = 'draft'
		ORDER BY started_at DESC LIMIT 1
	`, companyID).Scan(&existing)

	switch {
	case err == nil:
		return existing, nil
	case err != sql.ErrNoRows:
		return 0, err
	}

	var id int64
	err = s.db.QueryRow(`
		INSERT INTO stocktakes (company_id, user_id, note)
		VALUES ($1, $2, $3) RETURNING id
	`, companyID, userID, strings.TrimSpace(note)).Scan(&id)
	return id, err
}

// Count records what was found for one item.
//
// The expected figure is read now and stored with the line, not looked up when the
// count is applied. It is what the person had in front of them with the item in their
// hands, and keeping it is what lets the variance still mean something tomorrow.
func (s *StocktakeService) Count(companyID, stocktakeID, itemID int64, counted int) error {
	if counted < 0 {
		return StocktakeInputError{"A count cannot be negative."}
	}

	var status string
	err := s.db.QueryRow(
		`SELECT status FROM stocktakes WHERE id = $1 AND company_id = $2`,
		stocktakeID, companyID,
	).Scan(&status)
	if err == sql.ErrNoRows {
		return StocktakeInputError{"That count isn't this company's."}
	}
	if err != nil {
		return err
	}
	if status != "draft" {
		return StocktakeInputError{"That count has already been finished."}
	}

	var expected int
	err = s.db.QueryRow(
		`SELECT quantity FROM items WHERE id = $1 AND company_id = $2`,
		itemID, companyID,
	).Scan(&expected)
	if err == sql.ErrNoRows {
		return StocktakeInputError{"That item isn't in this company's catalogue."}
	}
	if err != nil {
		return err
	}

	// Counting the same item twice replaces the first figure — somebody recounting a
	// shelf is correcting themselves, not adding to it.
	_, err = s.db.Exec(`
		INSERT INTO stocktake_lines (stocktake_id, item_id, expected, counted)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (stocktake_id, item_id)
		DO UPDATE SET expected = EXCLUDED.expected,
		              counted = EXCLUDED.counted,
		              counted_at = NOW()
	`, stocktakeID, itemID, expected, counted)
	return err
}

// Get reads a count and its lines.
func (s *StocktakeService) Get(companyID, stocktakeID int64) (Stocktake, error) {
	var out Stocktake
	var note, appliedAt sql.NullString

	err := s.db.QueryRow(`
		SELECT id, status, COALESCE(note, ''),
		       TO_CHAR(started_at, 'YYYY-MM-DD"T"HH24:MI:SSZ'),
		       TO_CHAR(applied_at, 'YYYY-MM-DD"T"HH24:MI:SSZ')
		FROM stocktakes WHERE id = $1 AND company_id = $2
	`, stocktakeID, companyID).Scan(&out.ID, &out.Status, &note, &out.StartedAt, &appliedAt)
	if err == sql.ErrNoRows {
		return out, StocktakeInputError{"That count isn't this company's."}
	}
	if err != nil {
		return out, err
	}
	out.Note = note.String
	out.AppliedAt = appliedAt.String

	rows, err := s.db.Query(`
		SELECT l.item_id, i.name, COALESCE(i.unit, ''), l.expected, l.counted
		FROM stocktake_lines l
		JOIN items i ON i.id = l.item_id
		WHERE l.stocktake_id = $1
		ORDER BY lower(i.name), l.item_id
	`, stocktakeID)
	if err != nil {
		return out, err
	}
	defer rows.Close()

	out.Lines = []StocktakeLine{}
	for rows.Next() {
		var line StocktakeLine
		if err := rows.Scan(
			&line.ItemID, &line.ItemName, &line.Unit, &line.Expected, &line.Counted,
		); err != nil {
			return out, err
		}
		out.ItemsCounted++
		if line.Variance() != 0 {
			out.ItemsOff++
		}
		out.Lines = append(out.Lines, line)
	}
	return out, rows.Err()
}

// Apply writes the count into the stock figures.
//
// Each item moves by the variance that was found — counted minus what the books said at
// the moment of counting — rather than being set to the counted number outright.
//
// That matters because counting a shop takes an hour, and the shop stays open. If three
// boxes are sold while the floor is being counted, setting stock to the counted figure
// would quietly undo those sales; applying the variance leaves them intact and still
// corrects what the count found. The difference only shows up on a busy afternoon, which
// is exactly when nobody would notice it going wrong.
func (s *StocktakeService) Apply(companyID, userID, stocktakeID int64) (int, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	var status string
	err = tx.QueryRow(`
		SELECT status FROM stocktakes WHERE id = $1 AND company_id = $2 FOR UPDATE
	`, stocktakeID, companyID).Scan(&status)
	if err == sql.ErrNoRows {
		return 0, StocktakeInputError{"That count isn't this company's."}
	}
	if err != nil {
		return 0, err
	}
	if status != "draft" {
		return 0, StocktakeInputError{"That count has already been finished."}
	}

	rows, err := tx.Query(`
		SELECT item_id, expected, counted FROM stocktake_lines
		WHERE stocktake_id = $1 AND counted <> expected
	`, stocktakeID)
	if err != nil {
		return 0, err
	}

	type adjustment struct {
		itemID int64
		change int
	}
	var adjustments []adjustment
	for rows.Next() {
		var itemID int64
		var expected, counted int
		if err := rows.Scan(&itemID, &expected, &counted); err != nil {
			rows.Close()
			return 0, err
		}
		adjustments = append(adjustments, adjustment{itemID: itemID, change: counted - expected})
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}

	for _, adj := range adjustments {
		var previous, updated int
		if err := tx.QueryRow(`
			UPDATE items
			SET quantity = GREATEST(quantity + $1, 0), updated_at = NOW()
			WHERE id = $2 AND company_id = $3
			RETURNING quantity - $1, quantity
		`, adj.change, adj.itemID, companyID).Scan(&previous, &updated); err != nil {
			return 0, err
		}

		if _, err := tx.Exec(`
			INSERT INTO stock_movements
				(item_id, company_id, user_id, movement_type, quantity_change,
				 previous_quantity, new_quantity, reference, note)
			VALUES ($1,$2,$3,'stocktake',$4,$5,$6,$7,$8)
		`, adj.itemID, companyID, userID, adj.change, previous, updated,
			"Stocktake", "Counted on the floor"); err != nil {
			return 0, err
		}
	}

	if _, err := tx.Exec(`
		UPDATE stocktakes SET status = 'applied', applied_at = NOW() WHERE id = $1
	`, stocktakeID); err != nil {
		return 0, err
	}

	return len(adjustments), tx.Commit()
}

// Abandon throws a count away without touching stock.
func (s *StocktakeService) Abandon(companyID, stocktakeID int64) error {
	result, err := s.db.Exec(`
		UPDATE stocktakes SET status = 'abandoned'
		WHERE id = $1 AND company_id = $2 AND status = 'draft'
	`, stocktakeID, companyID)
	if err != nil {
		return err
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return StocktakeInputError{"That count isn't this company's, or it is already finished."}
	}
	return nil
}
