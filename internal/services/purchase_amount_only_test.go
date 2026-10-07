package services

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"os"
	"testing"
	"time"
)

func TestPurchaseAmountOnlyValidation(t *testing.T) {
	ptr := func(v float64) *float64 { return &v }
	tests := []struct {
		name   string
		amount *float64
		items  []PurchaseLine
		paid   float64
	}{
		{"missing", nil, nil, 0},
		{"zero", ptr(0), nil, 0},
		{"negative", ptr(-1), nil, 0},
		{"rounds to zero", ptr(0.001), nil, 0},
		{"too large", ptr(1e10), nil, 0},
		{"infinite", ptr(math.Inf(1)), nil, 0},
		{"nan", ptr(math.NaN()), nil, 0},
		{"mixed modes", ptr(100), []PurchaseLine{{ItemID: 1, Qty: 1, Rate: 100}}, 0},
		{"negative payment", ptr(100), nil, -1},
		{"invalid payment", ptr(100), nil, math.NaN()},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Invalid input must be rejected before opening any transaction.
			_, err := NewPurchaseService(nil).RecordBill(context.Background(), 1, 1, PurchaseBillRequest{
				SupplierID: 1, BillNumber: "TEST", BillAmount: tc.amount, Items: tc.items, PaidAmount: tc.paid,
			})
			var input PurchaseInputError
			if !errors.As(err, &input) {
				t.Fatalf("expected input error, got %v", err)
			}
		})
	}
}

func TestAmountOnlyPurchaseLedger(t *testing.T) {
	dsn := os.Getenv("INVO_TEST_DB")
	if dsn == "" {
		t.Skip("set INVO_TEST_DB to run purchase ledger integration checks")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := db.Exec(query, args...); err != nil {
			t.Fatal(err)
		}
	}
	schema := fmt.Sprintf("purchase_amount_test_%d", time.Now().UnixNano())
	exec("CREATE SCHEMA " + schema)
	defer func() { _, _ = db.Exec("DROP SCHEMA " + schema + " CASCADE") }()
	exec("SET search_path TO " + schema)
	exec(`CREATE TABLE companies (id BIGINT PRIMARY KEY);
	 CREATE TABLE items (id BIGINT PRIMARY KEY, company_id BIGINT, name TEXT,
	 quantity INTEGER, cost_price NUMERIC, updated_at TIMESTAMPTZ);
	 CREATE TABLE stock_movements (item_id BIGINT, company_id BIGINT, user_id BIGINT,
	 movement_type TEXT, quantity_change INTEGER, previous_quantity INTEGER,
	 new_quantity INTEGER, reference TEXT, note TEXT);
	 INSERT INTO companies VALUES (1), (2);
	 INSERT INTO items VALUES (1, 1, 'Adhesive', 10, 90, NOW());`)
	for _, path := range []string{"../../migrations/55_purchases.up.sql", "../../migrations/62_purchase_returns.up.sql"} {
		migration, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		exec(string(migration))
	}
	exec(`INSERT INTO suppliers (id, company_id, user_id, name) VALUES
	 (1, 1, 1, 'Supplier A'), (2, 2, 2, 'Other company')`)
	svc := NewPurchaseService(db)
	ctx := context.Background()
	date := "2026-10-06"
	amount := 1250.0
	req := PurchaseBillRequest{SupplierID: 1, BillNumber: "AMT-01", BillDate: &date,
		BillAmount: &amount, PaidAmount: 250, PaidMethod: "UPI", Notes: "Invoice total only"}
	id, err := svc.RecordBill(ctx, 1, 1, req)
	if err != nil {
		t.Fatal(err)
	}
	checkBill := func(id int64, total, paid, remaining float64, status string) {
		t.Helper()
		var gotTotal, gotPaid, gotRemaining float64
		var gotStatus string
		if err := db.QueryRow("SELECT total, paid_amount, remaining_amount, status FROM purchase_bills WHERE id=$1", id).
			Scan(&gotTotal, &gotPaid, &gotRemaining, &gotStatus); err != nil {
			t.Fatal(err)
		}
		if gotTotal != total || gotPaid != paid || gotRemaining != remaining || gotStatus != status {
			t.Fatalf("bill %d: got %v/%v/%v %s", id, gotTotal, gotPaid, gotRemaining, gotStatus)
		}
	}
	checkBill(id, 1250, 250, 1000, "partial")
	var qty, itemRows, movementRows int
	var cost float64
	if err := db.QueryRow(`SELECT quantity, cost_price,
	 (SELECT COUNT(*) FROM purchase_bill_items), (SELECT COUNT(*) FROM stock_movements)
	 FROM items WHERE id=1`).Scan(&qty, &cost, &itemRows, &movementRows); err != nil {
		t.Fatal(err)
	}
	if qty != 10 || cost != 90 || itemRows != 0 || movementRows != 0 {
		t.Fatal("amount-only bill changed stock")
	}
	entries, err := svc.SupplierLedger(ctx, 1, 1, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].Reference != "AMT-01" || entries[0].Debit != 1250 || entries[1].Balance != 1000 {
		t.Fatalf("unexpected ledger: %+v", entries)
	}
	if _, err := svc.RecordBill(ctx, 1, 1, req); err == nil {
		t.Fatal("duplicate invoice accepted")
	}
	req.SupplierID = 2
	if _, err := svc.RecordBill(ctx, 1, 1, req); err == nil {
		t.Fatal("another company's supplier accepted")
	}
	req.SupplierID = 1
	req.BillNumber = "OVERPAID"
	req.PaidAmount = 1251
	if _, err := svc.RecordBill(ctx, 1, 1, req); err == nil {
		t.Fatal("overpayment accepted")
	}
	if _, err := svc.PaySupplier(ctx, 1, SupplierPaymentRequest{SupplierID: 1, BillID: &id, Amount: 1000, Method: "Cash", PaidOn: &date}); err != nil {
		t.Fatal(err)
	}
	checkBill(id, 1250, 1250, 0, "paid")
	// An advance must settle an amount-only bill exactly as it settles a stock bill.
	if _, err := svc.PaySupplier(ctx, 1, SupplierPaymentRequest{SupplierID: 1, Amount: 300, Method: "UPI", PaidOn: &date}); err != nil {
		t.Fatal(err)
	}
	amount = 500
	req.BillNumber, req.PaidAmount = "AMT-02", 0
	id, err = svc.RecordBill(ctx, 1, 1, req)
	if err != nil {
		t.Fatal(err)
	}
	checkBill(id, 500, 300, 200, "partial")
	// The old item-based request still adds stock and calculates GST.
	stockID, err := svc.RecordBill(ctx, 1, 1, PurchaseBillRequest{
		SupplierID: 1, BillNumber: "STOCK-01", BillDate: &date,
		Items: []PurchaseLine{{ItemID: 1, Qty: 2, Rate: 100, TaxRate: 18}},
	})
	if err != nil {
		t.Fatal(err)
	}
	checkBill(stockID, 236, 0, 236, "unpaid")
	if err := db.QueryRow("SELECT quantity, cost_price FROM items WHERE id=1").Scan(&qty, &cost); err != nil {
		t.Fatal(err)
	}
	if qty != 12 || cost != 100 {
		t.Fatalf("stock path changed: quantity %d, cost %v", qty, cost)
	}
	totals, err := svc.SupplierLedgerTotals(ctx, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if totals.Billed != 1986 || totals.Paid != 1550 || totals.Balance != 436 {
		t.Fatalf("supplier totals do not reconcile: %+v", totals)
	}
}
