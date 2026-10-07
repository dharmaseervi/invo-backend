package services

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"reflect"
	"testing"
	"time"

	_ "github.com/lib/pq"
)

// Run against a throwaway Postgres database with INVO_TEST_DB. Each run uses its
// own schema; no existing application tables or data are changed.
func TestCashClosingSnapshot(t *testing.T) {
	dsn := os.Getenv("INVO_TEST_DB")
	if dsn == "" {
		t.Skip("set INVO_TEST_DB to run the cash-closing regression")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	defer db.Close()
	schema := fmt.Sprintf("closing_test_%d", time.Now().UnixNano())
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := db.ExecContext(context.Background(),
			query, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec("CREATE SCHEMA " + schema)
	defer func() {
		_, _ = db.ExecContext(context.Background(),
			"DROP SCHEMA "+schema+" CASCADE")
	}()
	exec("SET search_path TO " + schema)
	exec(`
		CREATE TABLE day_closings (
		 company_id BIGINT, user_id BIGINT, closing_date DATE,
		 opening_cash NUMERIC, cash_in NUMERIC, cash_out NUMERIC,
		 expected_cash NUMERIC, counted_cash NUMERIC, difference NUMERIC,
		 note TEXT, updated_at TIMESTAMP, UNIQUE(company_id, closing_date));
		CREATE TABLE payments (company_id BIGINT, payment_date DATE, amount NUMERIC,
		 status TEXT, payment_method TEXT);
		CREATE TABLE supplier_payments (company_id BIGINT, paid_on DATE, amount NUMERIC, method TEXT);
		CREATE TABLE refunds (company_id BIGINT, refund_date DATE, amount NUMERIC, method TEXT);
		CREATE TABLE expensess (company_id BIGINT, date DATE, amount NUMERIC, payment_method TEXT);
	`)
	migration, err := os.ReadFile("../../migrations/60_cash_closing_breakdowns.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	exec(string(migration))
	exec(`INSERT INTO day_closings
	 (company_id, closing_date, opening_cash, cash_in, cash_out, expected_cash, counted_cash, difference)
	 VALUES (1, '2026-10-01', 0, 1000, 0, 1000, 1000, 0)`)
	exec(`INSERT INTO payments VALUES
	 (1, '2026-10-02', 500, 'active', 'Cash'),
	 (1, '2026-10-02', 900, 'active', 'UPI'),
	 (1, '2026-10-02', 800, 'reversed', 'Cash'),
	 (2, '2026-10-02', 700, 'active', 'Cash');
	 INSERT INTO supplier_payments VALUES (1, '2026-10-02', 100, 'Cash');
	 INSERT INTO refunds VALUES (1, '2026-10-02', 50, 'Cash');
	 INSERT INTO expensess VALUES
	 (1, '2026-10-02', 25, 'Cash'), (1, '2026-10-02', 75, 'UPI'),
	 (1, '2026-10-02', 30, NULL)`)
	svc := NewLedgerService(db)
	open, err := svc.ClosingFor(context.Background(), 1, "2026-10-02")
	if err != nil {
		t.Fatal(err)
	}
	if open.Closed || open.Opening != 1000 || open.CashIn != 500 || open.CashOut != 175 || open.Expected != 1325 {
		t.Fatalf("incorrect open-day cash calculation: %+v", open)
	}
	saved, err := svc.Close(context.Background(), 1, 1, "2026-10-02", 1320, nil, "five short")
	if err != nil {
		t.Fatal(err)
	}
	if saved.Difference != -5 {
		t.Fatalf("difference = %v", saved.Difference)
	}
	// Corrections after closing must not alter the snapshot or its explanation.
	exec("UPDATE payments SET amount = amount + 200 WHERE company_id = 1")
	exec("DELETE FROM expensess WHERE payment_method = 'Cash'")
	reopened, err := svc.ClosingFor(context.Background(), 1, "2026-10-02")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(saved, reopened) {
		t.Fatalf("snapshot changed:\n saved %+v\n read %+v", saved, reopened)
	}
	recent, err := svc.RecentClosings(context.Background(), 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(recent) != 1 || recent[0].Expected != saved.Expected || recent[0].Difference != saved.Difference {
		t.Fatalf("history disagrees with saved day: %+v", recent)
	}
	// A deliberate recount can replace the snapshot, including an explicit zero float.
	zero := 0.0
	recount, err := svc.Close(context.Background(), 1, 1, "2026-10-02", 550, &zero, "recount")
	if err != nil {
		t.Fatal(err)
	}
	if recount.Opening != 0 || recount.Expected != 550 || recount.Difference != 0 {
		t.Fatalf("incorrect recount: %+v", recount)
	}
	reopened, err = svc.ClosingFor(context.Background(), 1, "2026-10-02")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(recount, reopened) {
		t.Fatal("recount breakdown was not saved")
	}
	// Older rows keep their recorded totals. No current transactions are guessed.
	legacy, err := svc.ClosingFor(context.Background(), 1, "2026-10-01")
	if err != nil {
		t.Fatal(err)
	}
	if !legacy.Closed || legacy.Expected != 1000 || len(legacy.InBreakdown) != 1 || legacy.InBreakdown[0].Amount != 1000 || legacy.InBreakdown[0].Count != 0 {
		t.Fatalf("invalid legacy snapshot: %+v", legacy)
	}
	// A different company must never see company 1's saved closing.
	other, err := svc.ClosingFor(context.Background(), 2, "2026-10-02")
	if err != nil {
		t.Fatal(err)
	}
	if other.Closed || other.Expected != 700 {
		t.Fatalf("company scope failed: %+v", other)
	}
}

func TestClosingBreakdownRejectsCorruption(t *testing.T) {
	if _, err := closingBreakdown([]byte(`{"unexpected":true}`), "Cash in", 100); err == nil {
		t.Fatal("invalid saved breakdown was silently accepted")
	}
	lines, err := closingBreakdown(nil, "Cash out", 0)
	if err != nil || len(lines) != 0 {
		t.Fatalf("empty legacy closing: %v %v", lines, err)
	}
}
