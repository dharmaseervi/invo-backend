// Package money does currency arithmetic in exact decimal instead of float64.
//
// Every money column in this schema is NUMERIC(n,2), so Postgres stores exact values
// and rounds on write. The drift came from Go: computing a line total, a tax figure and
// an invoice total as float64 lets each one land a fraction of a paisa off, and because
// line rows and invoice rows are rounded independently, SUM(invoice_items.total) could
// disagree with invoices.total by a cent. On a tax invoice that is a real defect — the
// printed document has to add up.
//
// The rule here: round at every step, and sum values that are already rounded, so the
// parts always reconstruct the whole exactly.
package money

import (
	"sort"

	"github.com/shopspring/decimal"
)

// places is the scale of every money column in the schema.
const places = 2

// Amount is an exact decimal currency value.
type Amount struct {
	d decimal.Decimal
}

// FromFloat converts a value arriving from JSON. Rounded immediately, because a
// float64 that looks like 12.34 may really be 12.339999999999999.
func FromFloat(f float64) Amount {
	return Amount{decimal.NewFromFloat(f).Round(places)}
}

// FromInt builds an amount from a whole currency unit.
func FromInt(i int64) Amount {
	return Amount{decimal.NewFromInt(i)}
}

// Zero is the additive identity.
func Zero() Amount {
	return Amount{decimal.Zero}
}

func (a Amount) Add(b Amount) Amount { return Amount{a.d.Add(b.d)} }
func (a Amount) Sub(b Amount) Amount { return Amount{a.d.Sub(b.d)} }

// MulQty multiplies by a whole quantity, so rate × qty stays exact.
func (a Amount) MulQty(qty int) Amount {
	return Amount{a.d.Mul(decimal.NewFromInt(int64(qty)))}
}

// TaxAt returns the tax due on this amount at a percentage rate, rounded to the
// currency's scale. ratePercent is a plain percentage: 18 means 18%.
func (a Amount) TaxAt(ratePercent float64) Amount {
	rate := decimal.NewFromFloat(ratePercent).Div(decimal.NewFromInt(100))
	return Amount{a.d.Mul(rate).Round(places)}
}

// Round snaps to the currency's scale, half away from zero, matching what NUMERIC(n,2)
// does on write. Applying it in Go keeps the value the code carries identical to the
// value the database will store.
func (a Amount) Round() Amount { return Amount{a.d.Round(places)} }

// ClampNonNegative floors an amount at zero. Used where a discount larger than the
// value it applies to would otherwise produce a negative line.
func (a Amount) ClampNonNegative() Amount {
	if a.d.IsNegative() {
		return Zero()
	}
	return a
}

// Min returns whichever amount is smaller, for capping a discount at the value it
// is being applied to.
func Min(a, b Amount) Amount {
	if a.d.LessThan(b.d) {
		return a
	}
	return b
}

func (a Amount) IsNegative() bool          { return a.d.IsNegative() }
func (a Amount) IsZero() bool              { return a.d.IsZero() }
func (a Amount) GreaterThan(b Amount) bool { return a.d.GreaterThan(b.d) }
func (a Amount) LessThan(b Amount) bool    { return a.d.LessThan(b.d) }
func (a Amount) Equal(b Amount) bool       { return a.d.Equal(b.d) }

// Float64 converts back for storage and JSON. Safe because the value is already
// rounded to two places and the destination column is NUMERIC(n,2).
func (a Amount) Float64() float64 {
	f, _ := a.d.Round(places).Float64()
	return f
}

// String renders the amount at full precision, for logs and error messages.
func (a Amount) String() string { return a.d.StringFixed(places) }

// Apportion splits total across weights in proportion to each weight, guaranteeing the
// parts sum back to total exactly.
//
// This is how an invoice-level discount is spread over lines. Rounding each share
// independently loses or gains paisa — ₹1000 across three equal lines is 333.33 three
// times, a paisa short — and on a tax invoice the parts have to reconstruct the whole.
// The remainder is therefore distributed a paisa at a time to the lines with the
// largest truncated fraction (the largest-remainder method), so the shortfall lands
// where it was most nearly earned.
//
// A zero total, or weights summing to zero, yields all-zero shares.
func Apportion(total Amount, weights []Amount) []Amount {
	shares := make([]Amount, len(weights))
	for i := range shares {
		shares[i] = Zero()
	}
	if len(weights) == 0 || total.IsZero() {
		return shares
	}

	sum := Zero()
	for _, w := range weights {
		sum = sum.Add(w)
	}
	if sum.IsZero() || sum.IsNegative() {
		return shares
	}

	// Truncate each exact share down, tracking what each line lost to truncation.
	type remainder struct {
		index int
		frac  decimal.Decimal
	}
	remainders := make([]remainder, 0, len(weights))
	allocated := Zero()

	for i, w := range weights {
		exact := total.d.Mul(w.d).Div(sum.d)
		truncated := exact.Truncate(places)
		shares[i] = Amount{truncated}
		allocated = allocated.Add(shares[i])
		remainders = append(remainders, remainder{index: i, frac: exact.Sub(truncated)})
	}

	// Hand out the leftover in single paisa, biggest truncated fraction first.
	leftover := total.Sub(allocated)
	step := Amount{decimal.New(1, -places)}

	sort.SliceStable(remainders, func(a, b int) bool {
		return remainders[a].frac.GreaterThan(remainders[b].frac)
	})

	for i := 0; leftover.GreaterThan(Zero()) && i < len(remainders); i++ {
		idx := remainders[i].index
		shares[idx] = shares[idx].Add(step)
		leftover = leftover.Sub(step)
	}

	return shares
}
