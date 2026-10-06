package services

import (
	"database/sql"
	"strconv"

	"invo-server/internal/models"
)

type CreditNoteService struct {
	db     *sql.DB
	ledger *LedgerService
}

func NewCreditNoteService(db *sql.DB, ledger *LedgerService) *CreditNoteService {
	return &CreditNoteService{db: db, ledger: ledger}
}

func (s *CreditNoteService) CreateTx(
	tx *sql.Tx,
	companyID int64,
	req models.CreditNoteRequestDTO,
) error {

	// 1️⃣ Validate input
	switch req.Type {
	case "return":
		if len(req.Items) == 0 {
			return CreditNoteInputError{"A return needs at least one item."}
		}
	case "adjustment", "discount":
		if req.Amount <= 0 {
			return CreditNoteInputError{"Enter the amount to credit."}
		}
	default:
		return CreditNoteInputError{"The credit note type must be return, adjustment or discount."}
	}

	// 1️⃣b Who and what it is against. The handler only checks the company, so a client
	// or invoice id from another business was accepted as long as the company was yours.
	// And an invoice must have been issued: a credit note against a draft cut the
	// draft's balance and could mark it paid before it was ever sent.
	var clientOK bool
	if err := tx.QueryRow(
		`SELECT EXISTS(SELECT 1 FROM clients WHERE id = $1 AND company_id = $2)`,
		req.ClientID, companyID,
	).Scan(&clientOK); err != nil {
		return err
	}
	if !clientOK {
		return CreditNoteInputError{"That client doesn't belong to this company."}
	}
	if req.InvoiceID != nil {
		var status string
		// FOR UPDATE: the "has this already been returned?" check below counts earlier
		// credit notes, and two returns arriving together both counted the same set and
		// both passed — so an invoice for 2 could be returned twice over, crediting the
		// customer twice and putting four items back into stock. Locking the invoice
		// makes the second one wait and then see the first one's credit note.
		err := tx.QueryRow(
			`SELECT status FROM invoices WHERE id = $1 AND company_id = $2 AND client_id = $3
			 FOR UPDATE`,
			*req.InvoiceID, companyID, req.ClientID,
		).Scan(&status)
		if err == sql.ErrNoRows {
			return CreditNoteInputError{"That invoice isn't one of this client's."}
		}
		if err != nil {
			return err
		}
		if status != "issued" && status != "partial" && status != "paid" {
			return CreditNoteInputError{"A credit note can only be raised against an issued invoice, not a " + status + " one."}
		}
	}

	// 1️⃣c Every returned line must be this company's own item, with a sane quantity
	// and rate.
	//
	// Nothing checked this before. The item ids were written to credit_note_items and
	// step 6 then added the quantity to whatever row carried that id, so a signed-in
	// user could name another business's item and change its stock — and reading the
	// credit note back returned that item's name. Ownership is checked here, and the
	// stock update in step 6 is scoped to the company as well, so neither the ids nor
	// a later change to this code can reach another business's inventory.
	if req.Type == "return" {
		for _, it := range req.Items {
			if it.Qty <= 0 {
				return CreditNoteInputError{"A returned line needs a quantity of at least one."}
			}
			if it.Rate < 0 {
				return CreditNoteInputError{"A returned line cannot have a negative rate."}
			}
			if it.TaxRate < 0 || it.TaxRate > 100 {
				return CreditNoteInputError{"A returned line's GST rate must be between 0 and 100."}
			}
			var itemOK bool
			if err := tx.QueryRow(
				`SELECT EXISTS(SELECT 1 FROM items WHERE id = $1 AND company_id = $2)`,
				it.ItemID, companyID,
			).Scan(&itemOK); err != nil {
				return err
			}
			if !itemOK {
				return CreditNoteInputError{"One of the returned items doesn't belong to this company."}
			}
		}
	}

	// 1️⃣d A return against an invoice cannot exceed what that invoice sold, counting
	// what earlier credit notes already took back.
	//
	// Without this a return of 100 against an invoice for 2 was accepted: 100 went into
	// stock that was never sold, the customer was credited for them, and repeating the
	// same return credited it again.
	if req.Type == "return" && req.InvoiceID != nil {
		// Several lines can name the same item, so they are summed before comparing —
		// two lines of 3 against 4 sold is an over-return even though neither line is.
		wanted := map[int64]float64{}
		for _, it := range req.Items {
			wanted[it.ItemID] += it.Qty
		}
		for itemID, qty := range wanted {
			var sold, returned float64
			if err := tx.QueryRow(
				`SELECT COALESCE(SUM(qty), 0) FROM invoice_items WHERE invoice_id = $1 AND item_id = $2`,
				*req.InvoiceID, itemID,
			).Scan(&sold); err != nil {
				return err
			}
			if sold == 0 {
				return CreditNoteInputError{"One of the returned items isn't on that invoice."}
			}
			if err := tx.QueryRow(`
				SELECT COALESCE(SUM(cni.qty), 0)
				FROM credit_note_items cni
				JOIN credit_notes cn ON cn.id = cni.credit_note_id
				WHERE cn.invoice_id = $1 AND cni.item_id = $2 AND cn.status <> 'cancelled'
			`, *req.InvoiceID, itemID).Scan(&returned); err != nil {
				return err
			}
			if qty+returned > sold {
				left := sold - returned
				if left <= 0 {
					return CreditNoteInputError{"Everything that invoice sold of one of these items has already been returned."}
				}
				return CreditNoteInputError{
					"That invoice only has " + strconv.FormatFloat(left, 'f', -1, 64) + " of one of these items left to return.",
				}
			}
		}
	}

	// 2️⃣ Calculate totals
	var subtotal, tax, total float64

	if req.Type == "return" {
		for _, it := range req.Items {
			lineBase := it.Qty * it.Rate
			lineTax := lineBase * it.TaxRate / 100

			subtotal += lineBase
			tax += lineTax
		}
		total = subtotal + tax
	} else {
		subtotal = req.Amount
		tax = 0
		total = subtotal
	}

	// 3️⃣ Generate credit number
	var creditNumber string
	err := tx.QueryRow(`
		SELECT 'CN-' || TO_CHAR(NOW(),'YYYY') || '-' ||
		       LPAD(nextval('credit_note_seq')::text,5,'0')
	`).Scan(&creditNumber)
	if err != nil {
		return err
	}

	// 4️⃣ Insert credit note
	var cnID int64
	err = tx.QueryRow(`
		INSERT INTO credit_notes (
			company_id, client_id, invoice_id,
			credit_number, type, reason, credit_date,
			subtotal, tax, total, balance, status
		)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$10,'issued')
		RETURNING id
	`,
		companyID,
		req.ClientID,
		req.InvoiceID,
		creditNumber,
		req.Type,
		req.Reason,
		req.CreditDate,
		subtotal,
		tax,
		total,
	).Scan(&cnID)

	if err != nil {
		return err
	}

	// 5️⃣ Insert items (return only)
	if req.Type == "return" {
		for _, it := range req.Items {
			lineBase := it.Qty * it.Rate
			lineTax := lineBase * it.TaxRate / 100

			_, err = tx.Exec(`
				INSERT INTO credit_note_items
					(credit_note_id, item_id, qty, rate, tax_rate, total)
				VALUES ($1,$2,$3,$4,$5,$6)
			`,
				cnID,
				it.ItemID,
				it.Qty,
				it.Rate,
				it.TaxRate,
				lineBase+lineTax,
			)
			if err != nil {
				return err
			}
		}
	}

	// 6️⃣ Put the returned goods back into stock.
	//
	// A return credit note previously touched only the ledger, so the customer's
	// balance was corrected while the goods they sent back stayed missing from
	// inventory. The movement is logged rather than silently adjusted, so the stock
	// audit trail explains where the quantity came from.
	if req.Type == "return" {
		if _, err = tx.Exec(`
			UPDATE items it
			SET quantity = it.quantity + agg.total_qty
			FROM (
				SELECT item_id, SUM(qty) AS total_qty
				FROM credit_note_items WHERE credit_note_id = $1 GROUP BY item_id
			) agg
			WHERE it.id = agg.item_id AND it.company_id = $2
		`, cnID, companyID); err != nil {
			return err
		}

		if _, err = tx.Exec(`
			INSERT INTO stock_movements (item_id, company_id, user_id, movement_type, quantity_change, previous_quantity, new_quantity, reference, note)
			SELECT it.id, $2, (SELECT user_id FROM companies WHERE id = $2),
			       'adjustment', agg.total_qty, it.quantity - agg.total_qty, it.quantity, $3, 'Goods returned'
			FROM items it
			JOIN (
				SELECT item_id, SUM(qty) AS total_qty
				FROM credit_note_items WHERE credit_note_id = $1 GROUP BY item_id
			) agg ON agg.item_id = it.id
			WHERE it.company_id = $2
		`, cnID, companyID, creditNumber); err != nil {
			return err
		}
	}

	// 7️⃣ Reduce what the referenced invoice still owes.
	//
	// Without this the aging report kept showing the full amount as outstanding even
	// after a full return, because it filters on remaining_amount > 0. Capped at the
	// outstanding balance so a credit note larger than the invoice cannot drive it
	// negative, and the status follows the new balance.
	if req.InvoiceID != nil {
		if _, err = tx.Exec(`
			UPDATE invoices
			SET remaining_amount = GREATEST(remaining_amount - $1, 0),
			    status = CASE
			        WHEN GREATEST(remaining_amount - $1, 0) <= 0 THEN 'paid'
			        ELSE status
			    END,
			    updated_at = NOW()
			WHERE id = $2 AND company_id = $3
		`, total, *req.InvoiceID, companyID); err != nil {
			return err
		}
	}

	// 8️⃣ Ledger entry
	narration := "Credit note issued"
	if req.Type == "discount" {
		narration = "Discount credit note issued"
	}

	return s.ledger.AddEntryTx(
		tx,
		companyID,
		req.ClientID,
		"CREDIT_NOTE",
		cnID,
		0,
		total,
		narration,
	)
}

// GetAll lists a company's credit notes, newest first.
//
// limit of 0 means the whole list, which is what the apps already in the store ask for;
// a caller that pages gets a stable order, since credit_date alone is not unique.
func (s *CreditNoteService) GetAll(
	companyID int64,
	search string,
	creditType string,
	limit, offset int,
) ([]models.CreditNoteListDTO, error) {

	query := `
		SELECT
			cn.id,
			cn.credit_number,
			cn.client_id,
			cl.name,
			cn.type,
			cn.total,
			cn.balance,
			cn.status,
			cn.credit_date
		FROM credit_notes cn
		JOIN clients cl ON cl.id = cn.client_id
		WHERE cn.company_id = $1
	`
	args := []interface{}{companyID}

	// Searching and filtering here, not in the app: the screen holds a page, so a
	// search done there missed every credit note that had not been downloaded.
	if search != "" {
		query += " AND (cn.credit_number ILIKE $2 OR cl.name ILIKE $2)"
		args = append(args, "%"+search+"%")
	}
	if creditType == "return" || creditType == "adjustment" || creditType == "discount" {
		// From a fixed list, never the raw parameter.
		query += " AND cn.type = '" + creditType + "'"
	}

	query += " ORDER BY cn.credit_date DESC, cn.id DESC"
	if limit > 0 {
		query += " LIMIT $" + strconv.Itoa(len(args)+1) + " OFFSET $" + strconv.Itoa(len(args)+2)
		args = append(args, limit, offset)
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := []models.CreditNoteListDTO{}

	for rows.Next() {
		var r models.CreditNoteListDTO
		if err := rows.Scan(
			&r.ID,
			&r.CreditNumber,
			&r.ClientID,
			&r.ClientName,
			&r.Type,
			&r.Total,
			&r.Balance,
			&r.Status,
			&r.CreditDate,
		); err != nil {
			return nil, err
		}
		result = append(result, r)
	}

	return result, nil
}

func (s *CreditNoteService) GetByID(
	tx *sql.Tx,
	companyID int64,
	creditNoteID int64,
) (*models.CreditNoteDetailResponse, error) {

	var cn models.CreditNoteDetailResponse

	err := tx.QueryRow(`
		SELECT
			cn.id,
			cn.credit_number,
			cn.client_id,
			cl.name,
			cn.invoice_id,
			i.invoice_number,
			cn.type,
			cn.reason,
			cn.subtotal,
			cn.tax,
			cn.total,
			cn.balance,
			cn.status,
			cn.credit_date,
			cn.created_at
		FROM credit_notes cn
		JOIN clients cl ON cl.id = cn.client_id
		LEFT JOIN invoices i ON i.id = cn.invoice_id
		WHERE cn.id = $1 AND cn.company_id = $2
	`, creditNoteID, companyID).Scan(
		&cn.ID,
		&cn.CreditNumber,
		&cn.ClientID,
		&cn.ClientName,
		&cn.InvoiceID,
		&cn.InvoiceNumber,
		&cn.Type,
		&cn.Reason,
		&cn.Subtotal,
		&cn.Tax,
		&cn.Total,
		&cn.Balance,
		&cn.Status,
		&cn.CreditDate,
		&cn.CreatedAt,
	)

	if err != nil {
		return nil, err
	}

	// 🔴 Fetch items only for return type
	rows, err := tx.Query(`
		SELECT
			cni.id,
			cni.item_id,
			it.name,
			cni.qty,
			cni.rate,
			cni.tax_rate,
			cni.total
		FROM credit_note_items cni
		JOIN items it ON it.id = cni.item_id
		WHERE cni.credit_note_id = $1
	`, creditNoteID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var item models.CreditNoteItemResponse
		if err := rows.Scan(
			&item.ID,
			&item.ItemID,
			&item.ItemName,
			&item.Qty,
			&item.Rate,
			&item.TaxRate,
			&item.Total,
		); err != nil {
			return nil, err
		}
		cn.Items = append(cn.Items, item)
	}

	if cn.Items == nil {
		cn.Items = []models.CreditNoteItemResponse{}
	}

	return &cn, nil
}

// CreditNoteInputError is a refusal the person can act on; its message is shown as is.
// Anything else stays a generic failure, as before.
type CreditNoteInputError struct{ Msg string }

func (e CreditNoteInputError) Error() string { return e.Msg }

// Summary counts a company's credit notes and what they came to, over everything that
// matches rather than the page a screen happens to hold.
func (s *CreditNoteService) Summary(
	companyID int64,
	search string,
) (models.CreditNoteSummary, error) {
	var out models.CreditNoteSummary

	query := `
		SELECT
			COUNT(*),
			COUNT(*) FILTER (WHERE cn.type = 'return'),
			COUNT(*) FILTER (WHERE cn.type = 'adjustment'),
			COUNT(*) FILTER (WHERE cn.type = 'discount'),
			COALESCE(SUM(cn.total), 0),
			COALESCE(SUM(cn.balance), 0)
		FROM credit_notes cn
		JOIN clients cl ON cl.id = cn.client_id
		WHERE cn.company_id = $1
	`
	args := []interface{}{companyID}
	if search != "" {
		query += " AND (cn.credit_number ILIKE $2 OR cl.name ILIKE $2)"
		args = append(args, "%"+search+"%")
	}

	err := s.db.QueryRow(query, args...).Scan(
		&out.Total, &out.Returns, &out.Adjustments, &out.Discounts, &out.Amount, &out.Balance,
	)
	return out, err
}

// CreditNoteInputError already exists for problems the person can fix; applying a
// credit note reuses it.

// ApplyToInvoice puts a credit note's remaining balance against one of the customer's
// unpaid invoices.
//
// No ledger entry is written here, deliberately. Issuing the credit note already
// credited the customer for its full value — that is what a credit note is. Applying it
// decides which invoice the credit settles, and writing a second credit would hand the
// customer the same money twice and quietly shrink the shop's receivables.
//
// Both rows are locked while the figures are read and changed, so two people applying
// the same credit note at once cannot each spend the whole balance.
func (s *CreditNoteService) ApplyToInvoice(
	companyID, creditNoteID, invoiceID int64,
	amount float64,
) (applied float64, err error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	var balance float64
	var status string
	var clientID int64
	err = tx.QueryRow(`
		SELECT balance, status, client_id FROM credit_notes
		WHERE id = $1 AND company_id = $2
		FOR UPDATE
	`, creditNoteID, companyID).Scan(&balance, &status, &clientID)
	if err == sql.ErrNoRows {
		return 0, CreditNoteInputError{"That credit note isn't this company's."}
	}
	if err != nil {
		return 0, err
	}
	if status == "cancelled" {
		return 0, CreditNoteInputError{"That credit note has been cancelled."}
	}
	if balance <= 0 {
		return 0, CreditNoteInputError{"That credit note has nothing left on it."}
	}

	// The invoice has to be the same customer's. A credit note belongs to whoever
	// returned the goods; spending it on somebody else's bill would move money between
	// two customers' accounts with nothing recording that it happened.
	var remaining float64
	var invoiceStatus string
	err = tx.QueryRow(`
		SELECT COALESCE(remaining_amount, total - COALESCE(paid_amount, 0)), status
		FROM invoices
		WHERE id = $1 AND company_id = $2 AND client_id = $3
		FOR UPDATE
	`, invoiceID, companyID, clientID).Scan(&remaining, &invoiceStatus)
	if err == sql.ErrNoRows {
		return 0, CreditNoteInputError{"That invoice isn't this customer's."}
	}
	if err != nil {
		return 0, err
	}
	if invoiceStatus == "cancelled" {
		return 0, CreditNoteInputError{"That invoice has been cancelled."}
	}
	if invoiceStatus == "draft" {
		return 0, CreditNoteInputError{"Issue that invoice before putting credit against it."}
	}
	if remaining <= 0 {
		return 0, CreditNoteInputError{"That invoice is already settled."}
	}

	// Asked for nothing in particular: as much of the credit as the invoice can take.
	// That is what somebody means by "apply this to that".
	applied = amount
	if applied <= 0 {
		applied = balance
	}
	if applied > balance {
		return 0, CreditNoteInputError{"That's more than the credit note has left."}
	}
	if applied > remaining {
		applied = remaining
	}

	if _, err = tx.Exec(`
		UPDATE invoices
		SET paid_amount = COALESCE(paid_amount, 0) + $2,
		    remaining_amount = COALESCE(remaining_amount, total - COALESCE(paid_amount, 0)) - $2,
		    status = CASE
		        WHEN COALESCE(remaining_amount, total - COALESCE(paid_amount, 0)) - $2 <= 0
		        THEN 'paid' ELSE 'partial' END,
		    updated_at = NOW()
		WHERE id = $1
	`, invoiceID, applied); err != nil {
		return 0, err
	}

	if _, err = tx.Exec(`
		UPDATE credit_notes
		SET balance = balance - $2,
		    status = CASE WHEN balance - $2 <= 0 THEN 'settled' ELSE status END
		WHERE id = $1
	`, creditNoteID, applied); err != nil {
		return 0, err
	}

	return applied, tx.Commit()
}
