/**
 * Invoice arithmetic, mirroring internal/handlers/invoice_totals.go.
 *
 * The server is the authority — nothing here is sent to it. This exists so the form can
 * show a total while the user types, and it has to agree with the server to the paisa:
 * a preview that says ₹1,180.00 against a saved invoice of ₹1,179.99 reads as a bug in
 * the invoice, not in the preview.
 *
 * Everything is in integer paise. The Go side uses exact decimals; float64 rupees would
 * drift apart from it on exactly the long invoices where it matters.
 */

export type TotalsLine = {
  qty: number;
  rate: number;
  /** Per-line discount in rupees, capped at the line value. */
  discount: number;
  tax_rate: number;
};

export type ComputedLine = {
  /** Taxable value after both the line discount and the apportioned invoice discount. */
  net: number;
  tax: number;
  total: number;
};

export type Totals = {
  lines: ComputedLine[];
  /** Taxable value BEFORE the invoice-level discount, so the printed rows reconcile. */
  subtotal: number;
  discount: number;
  tax: number;
  total: number;
};

const paise = (rupees: number) => Math.round((Number(rupees) || 0) * 100);
const rupees = (p: number) => p / 100;

/**
 * Largest-remainder apportionment, as money.Apportion does it: truncate every share,
 * then hand out the leftover a paisa at a time, biggest lost fraction first. The parts
 * therefore add up to the total exactly rather than to a paisa either side of it.
 */
function apportion(totalPaise: number, weights: number[]): number[] {
  const shares = weights.map(() => 0);
  const sum = weights.reduce((a, b) => a + b, 0);
  if (!totalPaise || sum <= 0) return shares;

  const fracs: { index: number; frac: number }[] = [];
  let allocated = 0;

  weights.forEach((w, i) => {
    const exact = (totalPaise * w) / sum;
    const truncated = Math.trunc(exact);
    shares[i] = truncated;
    allocated += truncated;
    fracs.push({ index: i, frac: exact - truncated });
  });

  fracs.sort((a, b) => b.frac - a.frac);
  let leftover = totalPaise - allocated;
  for (let i = 0; leftover > 0 && i < fracs.length; i++) {
    shares[fracs[i].index] += 1;
    leftover -= 1;
  }
  return shares;
}

export function computeTotals(items: TotalsLine[], invoiceDiscount: number): Totals {
  const lines: { net: number; tax: number; total: number }[] = [];
  let subtotal = 0;
  let tax = 0;

  for (const item of items) {
    const base = Math.round(paise(item.rate) * (Number(item.qty) || 0));
    // Capped at the line value: a discount larger than the line would otherwise make
    // the line, and the invoice, negative.
    const lineDiscount = Math.min(Math.max(paise(item.discount), 0), base);
    const net = base - lineDiscount;
    const lineTax = Math.round((net * (Number(item.tax_rate) || 0)) / 100);

    lines.push({ net, tax: lineTax, total: net + lineTax });
    subtotal += net;
    tax += lineTax;
  }

  const discount = Math.min(Math.max(paise(invoiceDiscount), 0), subtotal);

  // Applied BEFORE tax and spread across the lines, per CGST s.15(3): a discount shown
  // on the invoice reduces the taxable value, so GST is due on the discounted amount.
  if (discount > 0) {
    const shares = apportion(
      discount,
      lines.map((l) => l.net),
    );
    tax = 0;
    lines.forEach((line, i) => {
      const taxable = line.net - shares[i];
      const lineTax = Math.round((taxable * (Number(items[i].tax_rate) || 0)) / 100);
      line.net = taxable;
      line.tax = lineTax;
      line.total = taxable + lineTax;
      tax += lineTax;
    });
  }

  return {
    lines: lines.map((l) => ({
      net: rupees(l.net),
      tax: rupees(l.tax),
      total: rupees(l.total),
    })),
    subtotal: rupees(subtotal),
    discount: rupees(discount),
    tax: rupees(tax),
    total: rupees(subtotal - discount + tax),
  };
}
