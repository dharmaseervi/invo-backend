package money

import "testing"

// The defining property: an invoice's parts must reconstruct its whole exactly. Line
// rows and invoice rows are rounded independently on write, so if the code sums
// unrounded values the printed document will not add up.
func TestRoundedLinesSumToRoundedTotal(t *testing.T) {
	// Rates chosen so naive float64 accumulation drifts.
	lines := []struct {
		rate    float64
		qty     int
		taxRate float64
	}{
		{0.1, 3, 18},
		{33.33, 3, 18},
		{19.99, 7, 5},
		{0.07, 11, 12},
	}

	subtotal, tax := Zero(), Zero()
	for _, l := range lines {
		lineNet := FromFloat(l.rate).MulQty(l.qty).Round()
		lineTax := lineNet.TaxAt(l.taxRate)
		subtotal = subtotal.Add(lineNet)
		tax = tax.Add(lineTax)
	}

	total := subtotal.Add(tax)

	// Reconstruct from the rounded per-line figures the way a reader adds up a printed
	// invoice; it must match to the paisa.
	var checkNet, checkTax float64
	for _, l := range lines {
		net := FromFloat(l.rate).MulQty(l.qty).Round()
		checkNet += net.Float64()
		checkTax += net.TaxAt(l.taxRate).Float64()
	}

	if got, want := subtotal.Float64(), checkNet; got != want {
		t.Errorf("subtotal: got %v, want %v", got, want)
	}
	if got, want := tax.Float64(), checkTax; got != want {
		t.Errorf("tax: got %v, want %v", got, want)
	}
	if got, want := total.Float64(), checkNet+checkTax; got != want {
		t.Errorf("total: got %v, want %v", got, want)
	}
}

// 0.1 + 0.2 is the canonical float64 failure. Written through variables on purpose:
// as untyped constants Go evaluates this at compile time in arbitrary precision and
// the bug does not appear, which is exactly how it hides in review.
func TestNoFloatingPointDrift(t *testing.T) {
	a, b := 0.1, 0.2
	if a+b == 0.3 {
		t.Fatal("expected float64 addition to drift; test cannot prove anything")
	}

	got := FromFloat(a).Add(FromFloat(b))
	if got.Float64() != 0.3 {
		t.Errorf("0.1 + 0.2: got %v, want 0.3", got.Float64())
	}

	// The same drift, accumulated the way an invoice sums many lines.
	var naive float64
	sum := Zero()
	for i := 0; i < 10; i++ {
		naive += 0.1
		sum = sum.Add(FromFloat(0.1))
	}
	if naive == 1.0 {
		t.Fatal("expected accumulated float64 drift; test cannot prove anything")
	}
	if sum.Float64() != 1.0 {
		t.Errorf("ten times 0.1: got %v, want 1.0", sum.Float64())
	}
}

func TestTaxRounding(t *testing.T) {
	cases := []struct {
		net     float64
		rate    float64
		wantTax float64
	}{
		{4500, 18, 810},
		{100, 5, 5},
		{33.33, 18, 6},   // 5.9994 rounds to 6.00
		{0.03, 18, 0.01}, // 0.0054 rounds to 0.01, half away from zero
		{1000, 2.5, 25},
	}
	for _, c := range cases {
		if got := FromFloat(c.net).TaxAt(c.rate).Float64(); got != c.wantTax {
			t.Errorf("tax on %v at %v%%: got %v, want %v", c.net, c.rate, got, c.wantTax)
		}
	}
}

// A discount must never be able to drive a line negative.
func TestDiscountCannotGoNegative(t *testing.T) {
	lineNet := FromFloat(100)
	discount := Min(FromFloat(250), lineNet)
	after := lineNet.Sub(discount).ClampNonNegative()

	if after.IsNegative() {
		t.Fatalf("line went negative: %v", after.Float64())
	}
	if after.Float64() != 0 {
		t.Errorf("over-discounted line: got %v, want 0", after.Float64())
	}
}

// Payment comparisons must not reject an amount that exactly settles an invoice.
func TestExactSettlementComparesEqual(t *testing.T) {
	remaining := FromFloat(0.1).Add(FromFloat(0.2)) // 0.30 exactly
	payment := FromFloat(0.3)

	if payment.GreaterThan(remaining) {
		t.Errorf("exact settlement rejected: payment %v > remaining %v", payment.Float64(), remaining.Float64())
	}
	if !payment.Equal(remaining) {
		t.Errorf("expected equality: %v vs %v", payment.Float64(), remaining.Float64())
	}
}

// The property that matters: shares must always sum back to the total exactly, however
// awkwardly it divides. A lost paisa here is an invoice that does not add up.
func TestApportionAlwaysSumsToTotal(t *testing.T) {
	cases := []struct {
		name    string
		total   float64
		weights []float64
	}{
		{"three equal lines, indivisible", 1000, []float64{100, 100, 100}},
		{"two equal lines, odd paisa", 0.01, []float64{50, 50}},
		{"uneven weights", 1000, []float64{4500, 300, 199.99}},
		{"one line takes all", 250.55, []float64{80}},
		{"seven lines", 100, []float64{1, 2, 3, 4, 5, 6, 7}},
		{"tiny total", 0.03, []float64{10, 20, 30, 40}},
		{"total equals sum", 500, []float64{200, 300}},
	}

	for _, c := range cases {
		weights := make([]Amount, len(c.weights))
		for i, w := range c.weights {
			weights[i] = FromFloat(w)
		}

		shares := Apportion(FromFloat(c.total), weights)

		sum := Zero()
		for _, s := range shares {
			if s.IsNegative() {
				t.Errorf("%s: negative share %v", c.name, s.Float64())
			}
			sum = sum.Add(s)
		}
		if !sum.Equal(FromFloat(c.total)) {
			t.Errorf("%s: shares sum to %v, want %v", c.name, sum.Float64(), c.total)
		}
	}
}

// Proportionality: a line worth twice as much absorbs twice the discount.
func TestApportionIsProportional(t *testing.T) {
	shares := Apportion(FromFloat(300), []Amount{FromFloat(100), FromFloat(200)})

	if shares[0].Float64() != 100 || shares[1].Float64() != 200 {
		t.Errorf("got %v and %v, want 100 and 200", shares[0].Float64(), shares[1].Float64())
	}
}

func TestApportionEdgeCases(t *testing.T) {
	if got := Apportion(FromFloat(100), nil); len(got) != 0 {
		t.Errorf("no weights: got %d shares, want 0", len(got))
	}

	zeroWeights := Apportion(FromFloat(100), []Amount{Zero(), Zero()})
	for i, s := range zeroWeights {
		if !s.IsZero() {
			t.Errorf("zero weights: share %d = %v, want 0", i, s.Float64())
		}
	}

	noDiscount := Apportion(Zero(), []Amount{FromFloat(10), FromFloat(20)})
	for i, s := range noDiscount {
		if !s.IsZero() {
			t.Errorf("zero total: share %d = %v, want 0", i, s.Float64())
		}
	}
}

// End to end: a ₹1000 invoice-level discount on three lines, applied before tax, must
// leave the lines reconstructing the invoice total to the paisa.
func TestInvoiceDiscountApportionedBeforeTax(t *testing.T) {
	nets := []Amount{FromFloat(4500), FromFloat(300), FromFloat(199.99)}
	taxRates := []float64{18, 18, 5}
	discount := FromFloat(1000)

	shares := Apportion(discount, nets)

	taxableSum, taxSum, lineTotalSum := Zero(), Zero(), Zero()
	for i, net := range nets {
		taxable := net.Sub(shares[i]).Round()
		tax := taxable.TaxAt(taxRates[i])
		taxableSum = taxableSum.Add(taxable)
		taxSum = taxSum.Add(tax)
		lineTotalSum = lineTotalSum.Add(taxable.Add(tax).Round())
	}

	grossNet := Zero()
	for _, n := range nets {
		grossNet = grossNet.Add(n)
	}

	// Header arithmetic the printed invoice shows.
	headerTotal := grossNet.Sub(discount).Add(taxSum).Round()

	if !taxableSum.Equal(grossNet.Sub(discount)) {
		t.Errorf("taxable %v != subtotal - discount %v", taxableSum.Float64(), grossNet.Sub(discount).Float64())
	}
	if !lineTotalSum.Equal(headerTotal) {
		t.Errorf("sum of line totals %v != header total %v", lineTotalSum.Float64(), headerTotal.Float64())
	}
}
