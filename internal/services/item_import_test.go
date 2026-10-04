package services

import "testing"

// The numbers a spreadsheet actually contains.
func TestParseMoneyAcceptsSpreadsheetFormatting(t *testing.T) {
	ok := map[string]float64{
		"":         0,
		"450":      450,
		"1,200.50": 1200.5,
		"₹1,200":   1200,
		"Rs 999":   999,
		"Rs.999":   999,
		" 1 200 ":  1200,
		"0":        0,
	}
	for in, want := range ok {
		got, err := parseMoney(in)
		if err != nil {
			t.Errorf("parseMoney(%q) returned %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("parseMoney(%q) = %v, want %v", in, got, want)
		}
	}
	for _, in := range []string{"abc", "-5", "12-", "N/A"} {
		if _, err := parseMoney(in); err == nil {
			t.Errorf("parseMoney(%q) was accepted", in)
		}
	}
}

// Stock is whole units. "12.0" is a spreadsheet writing 12; "12.5" is a real conflict
// with how stock is stored, so it has to be refused rather than rounded.
func TestParseCountRefusesFractionsButAcceptsTrailingZero(t *testing.T) {
	for in, want := range map[string]int{"": 0, "12": 12, "12.0": 12, "1,000": 1000} {
		got, err := parseCount(in)
		if err != nil {
			t.Errorf("parseCount(%q) returned %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("parseCount(%q) = %d, want %d", in, got, want)
		}
	}
	for _, in := range []string{"12.5", "-3", "two"} {
		if _, err := parseCount(in); err == nil {
			t.Errorf("parseCount(%q) was accepted", in)
		}
	}
}

// Column names differ in every export; the point is that nobody has to rename them.
func TestHeaderDetection(t *testing.T) {
	cases := map[string]ImportColumn{
		"Item Name": ColName, "PRODUCT NAME": ColName, "particulars": ColName,
		"Selling Price (₹)": ColPrice, "Rate": ColPrice, "MRP": ColPrice,
		"HSN/SAC Code": ColHSN, "Qty": ColQuantity, "Opening Stock": ColQuantity,
		"GST %": ColTaxRate, "Reorder Level": ColLowStock, "Purchase Rate": ColCost,
	}
	for header, want := range cases {
		if got := headerAliases[normaliseHeader(header)]; got != want {
			t.Errorf("%q detected as %q, want %q", header, got, want)
		}
	}
}
