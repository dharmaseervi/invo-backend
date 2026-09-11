package handlers

import (
	"fmt"

	"invo-server/internal/models"
	"invo-server/internal/money"
)

// invoiceLine holds one line's computed amounts, already rounded to the currency scale.
type invoiceLine struct {
	Discount money.Amount // line discount, capped at the line value
	Net      money.Amount // taxable value after both line and apportioned invoice discount
	Tax      money.Amount
	Total    money.Amount // Net + Tax, what invoice_items.total stores
}

// invoiceTotals is the header arithmetic. Subtotal is the taxable value BEFORE the
// invoice-level discount, so a printed invoice can show Subtotal, Discount, tax rows
// and Total, and have them reconcile: Subtotal - Discount + Tax == Total.
type invoiceTotals struct {
	Lines    []invoiceLine
	Subtotal money.Amount
	Discount money.Amount
	Tax      money.Amount
	Total    money.Amount
}

// computeInvoiceTotals is the single source of truth for invoice arithmetic, shared by
// creation and editing. These were previously two separate float64 implementations that
// could — and did — disagree.
//
// An invoice-level discount is apportioned across lines and applied BEFORE tax, per
// CGST s.15(3): a discount shown on the invoice reduces the transaction value, so tax
// is due on the discounted amount. Applying it after tax would declare GST on money
// never collected and leave the line rows unable to reconstruct the total, which is
// what GSTR-1 reads to derive taxable value per HSN.
func computeInvoiceTotals(items []models.InvoiceItemRequest, invoiceDiscount float64) (invoiceTotals, error) {
	var out invoiceTotals
	out.Lines = make([]invoiceLine, 0, len(items))
	out.Subtotal, out.Tax = money.Zero(), money.Zero()

	for i, item := range items {
		if item.Qty <= 0 {
			return out, fmt.Errorf("Line %d: quantity must be greater than zero", i+1)
		}
		if item.Rate < 0 || item.Discount < 0 {
			return out, fmt.Errorf("Line %d: rate and discount cannot be negative", i+1)
		}

		lineBase := money.FromFloat(item.Rate).MulQty(item.Qty).Round()

		// Capped at the line value: an unchecked discount larger than the line produced
		// a negative line total and an invoice owing less than nothing.
		lineDiscount := money.Min(money.FromFloat(item.Discount), lineBase)
		net := lineBase.Sub(lineDiscount).Round()
		tax := net.TaxAt(item.TaxRate)

		out.Lines = append(out.Lines, invoiceLine{
			Discount: lineDiscount,
			Net:      net,
			Tax:      tax,
			Total:    net.Add(tax).Round(),
		})
		out.Subtotal = out.Subtotal.Add(net)
		out.Tax = out.Tax.Add(tax)
	}

	out.Discount = money.Min(money.FromFloat(invoiceDiscount).ClampNonNegative(), out.Subtotal)

	if !out.Discount.IsZero() {
		weights := make([]money.Amount, len(out.Lines))
		for i := range out.Lines {
			weights[i] = out.Lines[i].Net
		}
		shares := money.Apportion(out.Discount, weights)

		out.Tax = money.Zero()
		for i := range out.Lines {
			taxable := out.Lines[i].Net.Sub(shares[i]).Round()
			tax := taxable.TaxAt(items[i].TaxRate)

			out.Lines[i].Net = taxable
			out.Lines[i].Tax = tax
			out.Lines[i].Total = taxable.Add(tax).Round()

			out.Tax = out.Tax.Add(tax)
		}
	}

	out.Total = out.Subtotal.Sub(out.Discount).Add(out.Tax).Round()
	return out, nil
}
