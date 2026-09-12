package handlers

import (
	"encoding/json"
	"math/rand"
	"os"
	"testing"

	"invo-server/internal/models"
)

// TestExportTotalsFixtures is a harness, not an assertion: it writes randomised cases
// and the server's own answer for each, so the web client's mirror of this arithmetic
// can be checked against it.
func TestExportTotalsFixtures(t *testing.T) {
	path := os.Getenv("TOTALS_FIXTURE_OUT")
	if path == "" {
		t.Skip("TOTALS_FIXTURE_OUT not set")
	}

	rng := rand.New(rand.NewSource(20260912))
	rates := []float64{0, 5, 12, 18, 28}

	type fixture struct {
		Items    []models.InvoiceItemRequest `json:"items"`
		Discount float64                     `json:"discount"`
		Subtotal float64                     `json:"subtotal"`
		DiscOut  float64                     `json:"discount_out"`
		Tax      float64                     `json:"tax"`
		Total    float64                     `json:"total"`
		Lines    []float64                   `json:"line_totals"`
	}

	out := []fixture{}
	for c := 0; c < 400; c++ {
		n := 1 + rng.Intn(6)
		items := make([]models.InvoiceItemRequest, n)
		for i := range items {
			items[i] = models.InvoiceItemRequest{
				ItemID:   1,
				Qty:      1 + rng.Intn(9),
				Rate:     float64(rng.Intn(500000)) / 100,
				Discount: float64(rng.Intn(5000)) / 100,
				TaxRate:  rates[rng.Intn(len(rates))],
			}
		}
		discount := float64(rng.Intn(20000)) / 100

		got, err := computeInvoiceTotals(items, discount)
		if err != nil {
			continue
		}
		f := fixture{
			Items:    items,
			Discount: discount,
			Subtotal: got.Subtotal.Float64(),
			DiscOut:  got.Discount.Float64(),
			Tax:      got.Tax.Float64(),
			Total:    got.Total.Float64(),
		}
		for _, l := range got.Lines {
			f.Lines = append(f.Lines, l.Total.Float64())
		}
		out = append(out, f)
	}

	data, _ := json.Marshal(out)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote %d fixtures", len(out))
}
