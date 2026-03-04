package services

import (
	"database/sql"
	"errors"
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
			return errors.New("items required for return credit note")
		}
	case "adjustment", "discount":
		if req.Amount <= 0 {
			return errors.New("amount required for credit note")
		}
	default:
		return errors.New("invalid credit note type")
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

	// 6️⃣ Ledger entry
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
func (s *CreditNoteService) GetAll(
	companyID int64,
) ([]models.CreditNoteListDTO, error) {

	rows, err := s.db.Query(`
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
		ORDER BY cn.credit_date DESC, cn.id DESC
	`, companyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []models.CreditNoteListDTO

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
