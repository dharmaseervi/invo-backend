package handlers

import (
	"testing"

	"invo-server/internal/models"
)

func line(qty int, rate, discount, taxRate float64) models.InvoiceItemRequest {
	return models.InvoiceItemRequest{Qty: qty, Rate: rate, Discount: discount, TaxRate: taxRate}
}

// The invariant a tax invoice must satisfy: the line rows reconstruct the header, and
// the header's own arithmetic closes. GSTR-1 derives taxable value from the line rows,
// so if these drift apart the return disagrees with the document given to the customer.
func assertReconciles(t *testing.T, name string, totals invoiceTotals) {
	t.Helper()

	sumLines := totals.Lines[0].Total
	for _, l := range totals.Lines[1:] {
		sumLines = sumLines.Add(l.Total)
	}
	if !sumLines.Equal(totals.Total) {
		t.Errorf("%s: lines sum to %v but total is %v", name, sumLines.Float64(), totals.Total.Float64())
	}

	header := totals.Subtotal.Sub(totals.Discount).Add(totals.Tax).Round()
	if !header.Equal(totals.Total) {
		t.Errorf("%s: subtotal-discount+tax = %v but total is %v", name, header.Float64(), totals.Total.Float64())
	}
}

// This is the live defect found on invoice INV/FY26-27/0003: a ₹1000 invoice discount
// where tax had been charged on the pre-discount value.
func TestInvoiceDiscountReducesTaxableValue(t *testing.T) {
	totals, err := computeInvoiceTotals([]models.InvoiceItemRequest{
		line(1, 4500, 0, 18),
	}, 1000)
	if err != nil {
		t.Fatal(err)
	}

	// Taxable value is 4500 - 1000 = 3500, so GST is 18% of 3500, not of 4500.
	if got, want := totals.Tax.Float64(), 630.0; got != want {
		t.Errorf("tax: got %v, want %v (18%% of the discounted 3500)", got, want)
	}
	if got, want := totals.Total.Float64(), 4130.0; got != want {
		t.Errorf("total: got %v, want %v", got, want)
	}
	assertReconciles(t, "single line with invoice discount", totals)
}

// A discount that divides unevenly across lines must not lose a paisa. These three
// lines total 733.30, and 100 across them does not divide cleanly at any scale.
func TestUnevenDiscountStillReconciles(t *testing.T) {
	items := []models.InvoiceItemRequest{
		line(3, 100, 0, 18),
		line(1, 199.99, 0, 5),
		line(7, 33.33, 0, 12),
	}

	totals, err := computeInvoiceTotals(items, 100)
	if err != nil {
		t.Fatal(err)
	}
	assertReconciles(t, "uneven three-line discount", totals)

	if got := totals.Discount.Float64(); got != 100 {
		t.Errorf("discount recorded as %v, want 100", got)
	}

	// Mixed tax rates: each line is taxed at its own rate on its discounted value, so
	// the totals cannot be reproduced by taxing the invoice as a whole.
	if totals.Tax.IsZero() {
		t.Error("expected tax across mixed rates")
	}
}

// Every paisa of a discount must reach some line, at any awkward amount.
func TestDiscountFullyDistributedAcrossLines(t *testing.T) {
	items := []models.InvoiceItemRequest{
		line(3, 100, 0, 18),
		line(1, 199.99, 0, 5),
		line(7, 33.33, 0, 12),
	}

	for _, discount := range []float64{0.01, 0.07, 33.33, 99.99, 100, 733.29, 733.30} {
		totals, err := computeInvoiceTotals(items, discount)
		if err != nil {
			t.Fatal(err)
		}
		if got := totals.Discount.Float64(); got != discount {
			t.Errorf("discount %v: recorded as %v", discount, got)
		}
		assertReconciles(t, "discount "+totals.Discount.String(), totals)
	}
}

func TestNoDiscountIsUnaffected(t *testing.T) {
	totals, err := computeInvoiceTotals([]models.InvoiceItemRequest{
		line(1, 4500, 0, 18),
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := totals.Tax.Float64(), 810.0; got != want {
		t.Errorf("tax: got %v, want %v", got, want)
	}
	if got, want := totals.Total.Float64(), 5310.0; got != want {
		t.Errorf("total: got %v, want %v", got, want)
	}
	assertReconciles(t, "no discount", totals)
}

// A line discount larger than the line must floor at zero, never go negative.
func TestOverLargeLineDiscountFloorsAtZero(t *testing.T) {
	totals, err := computeInvoiceTotals([]models.InvoiceItemRequest{
		line(1, 100, 250, 18),
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if totals.Total.IsNegative() || totals.Subtotal.IsNegative() {
		t.Fatalf("negative invoice: subtotal %v total %v", totals.Subtotal.Float64(), totals.Total.Float64())
	}
	if got := totals.Lines[0].Discount.Float64(); got != 100 {
		t.Errorf("stored discount %v, want it capped at the line value 100", got)
	}
}

// An invoice discount exceeding the whole invoice is capped, not negative.
func TestOverLargeInvoiceDiscountIsCapped(t *testing.T) {
	totals, err := computeInvoiceTotals([]models.InvoiceItemRequest{
		line(1, 100, 0, 18),
	}, 5000)
	if err != nil {
		t.Fatal(err)
	}
	if got := totals.Discount.Float64(); got != 100 {
		t.Errorf("discount %v, want capped at subtotal 100", got)
	}
	if got := totals.Total.Float64(); got != 0 {
		t.Errorf("total %v, want 0", got)
	}
}

func TestRejectsInvalidLines(t *testing.T) {
	cases := []struct {
		name  string
		items []models.InvoiceItemRequest
	}{
		{"zero quantity", []models.InvoiceItemRequest{line(0, 100, 0, 18)}},
		{"negative quantity", []models.InvoiceItemRequest{line(-2, 100, 0, 18)}},
		{"negative rate", []models.InvoiceItemRequest{line(1, -100, 0, 18)}},
		{"negative discount", []models.InvoiceItemRequest{line(1, 100, -50, 18)}},
	}
	for _, c := range cases {
		if _, err := computeInvoiceTotals(c.items, 0); err == nil {
			t.Errorf("%s: expected rejection, got none", c.name)
		}
	}
}
