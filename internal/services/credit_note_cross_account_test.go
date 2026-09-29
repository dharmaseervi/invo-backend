package services

import (
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	"invo-server/internal/models"

	_ "github.com/lib/pq"
)

// A return credit note must never reach another company's stock.
//
// This is a database test rather than a unit test on purpose: the hole it guards was a
// missing WHERE clause, and only a real insert-and-read proves the clause is there. A
// signed-in user could name another business's item id in a return, which added that
// quantity to the other company's stock and disclosed the item's name when the credit
// note was read back.
//
// Needs a throwaway Postgres database, given as INVO_TEST_DB, e.g.
//
//	INVO_TEST_DB='host=localhost dbname=invo_test sslmode=disable' go test ./internal/services
//
// It is skipped when that is unset so the normal test run stays offline.
func TestReturnCreditNoteCannotTouchAnotherCompanysStock(t *testing.T) {
	dsn := os.Getenv("INVO_TEST_DB")
	if dsn == "" {
		t.Skip("set INVO_TEST_DB to run this against a throwaway database")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		t.Fatalf("ping: %v", err)
	}

	victim := newAccount(t, db, "victim")
	attacker := newAccount(t, db, "attacker")
	victimItem := newItem(t, db, victim, "Victim Stock", 40)
	attackerItem := newItem(t, db, attacker, "Attacker Stock", 5)

	svc := NewCreditNoteService(db, NewLedgerService(db))

	// The attack: the attacker's own company and client, the victim's item id.
	err = inTx(t, db, func(tx *sql.Tx) error {
		return svc.CreateTx(tx, attacker.companyID, models.CreditNoteRequestDTO{
			ClientID:   attacker.clientID,
			CompanyID:  attacker.companyID,
			Type:       "return",
			CreditDate: "2026-09-29",
			Items:      []models.CreditNoteItemDTO{{ItemID: victimItem, Qty: 500, Rate: 1}},
		})
	})
	if err == nil {
		t.Fatal("a return naming another company's item was accepted")
	}
	if _, ok := err.(CreditNoteInputError); !ok {
		t.Fatalf("want CreditNoteInputError, got %T: %v", err, err)
	}
	if got := quantityOf(t, db, victimItem); got != 40 {
		t.Fatalf("the victim's stock changed: want 40, got %d", got)
	}

	// Nonsense quantities and rates are refused too.
	for _, line := range []models.CreditNoteItemDTO{
		{ItemID: attackerItem, Qty: 0, Rate: 10},
		{ItemID: attackerItem, Qty: -3, Rate: 10},
		{ItemID: attackerItem, Qty: 1, Rate: -10},
		{ItemID: attackerItem, Qty: 1, Rate: 10, TaxRate: 900},
	} {
		err := inTx(t, db, func(tx *sql.Tx) error {
			return svc.CreateTx(tx, attacker.companyID, models.CreditNoteRequestDTO{
				ClientID: attacker.clientID, CompanyID: attacker.companyID,
				Type: "return", CreditDate: "2026-09-29",
				Items: []models.CreditNoteItemDTO{line},
			})
		})
		if err == nil {
			t.Fatalf("line %+v was accepted", line)
		}
	}

	// And the ordinary case still works: the attacker's own item goes back into stock.
	if err := inTx(t, db, func(tx *sql.Tx) error {
		return svc.CreateTx(tx, attacker.companyID, models.CreditNoteRequestDTO{
			ClientID: attacker.clientID, CompanyID: attacker.companyID,
			Type: "return", CreditDate: "2026-09-29",
			Items: []models.CreditNoteItemDTO{{ItemID: attackerItem, Qty: 3, Rate: 100, TaxRate: 18}},
		})
	}); err != nil {
		t.Fatalf("a legitimate return failed: %v", err)
	}
	if got := quantityOf(t, db, attackerItem); got != 8 {
		t.Fatalf("own stock after returning 3 of 5: want 8, got %d", got)
	}
}

type account struct {
	userID, companyID, clientID int64
}

// newAccount makes a user, a company and a client, and removes them when the test ends.
func newAccount(t *testing.T, db *sql.DB, label string) account {
	t.Helper()
	var a account
	// Unique per run, so a test that fails half way through cannot block the next one.
	unique := fmt.Sprintf("%s-%d", label, time.Now().UnixNano())
	email := unique + "@test.invalid"
	// Registered before the inserts, so anything created below is removed even if a
	// later insert fails (the company, client and items go with the user).
	t.Cleanup(func() { db.Exec(`DELETE FROM users WHERE email = $1`, email) })
	must(t, db.QueryRow(
		`INSERT INTO users (email, password_hash, is_verified, created_at, updated_at, tokens_valid_from)
		 VALUES ($1, 'x', true, NOW(), NOW(), NOW()) RETURNING id`, email).Scan(&a.userID))
	must(t, db.QueryRow(
		`INSERT INTO companies (user_id, name, state) VALUES ($1, $2, 'Karnataka') RETURNING id`,
		a.userID, unique+" Co").Scan(&a.companyID))
	must(t, db.QueryRow(
		`INSERT INTO clients (company_id, user_id, name, email) VALUES ($1, $2, $3, $4) RETURNING id`,
		a.companyID, a.userID, label+" Client", email).Scan(&a.clientID))
	return a
}

func newItem(t *testing.T, db *sql.DB, a account, name string, qty int) int64 {
	t.Helper()
	var id int64
	must(t, db.QueryRow(
		`INSERT INTO items (company_id, user_id, name, price, quantity, tax_rate)
		 VALUES ($1, $2, $3, 100, $4, 18) RETURNING id`,
		a.companyID, a.userID, name, qty).Scan(&id))
	return id
}

func quantityOf(t *testing.T, db *sql.DB, itemID int64) int {
	t.Helper()
	var qty int
	must(t, db.QueryRow(`SELECT quantity FROM items WHERE id = $1`, itemID).Scan(&qty))
	return qty
}

// inTx runs fn in one transaction, the way the handler does: committed when it
// succeeds, rolled back when it fails. What a refused call leaves behind is exactly
// what the test needs to check.
func inTx(t *testing.T, db *sql.DB, fn func(*sql.Tx) error) error {
	t.Helper()
	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	err = fn(tx)
	if err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
}
