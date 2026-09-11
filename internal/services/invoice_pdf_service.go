package services

import (
	"database/sql"
	"fmt"
	"strings"

	"invo-server/internal/pdf"
)

func FetchInvoicePDFData(
	db *sql.DB,
	invoiceID int,
) (pdf.InvoicePDFData, error) {

	var data pdf.InvoicePDFData

	/* -----------------------------
	   1️⃣ Fetch invoice + company
	------------------------------ */
	err := db.QueryRow(`
		SELECT
			i.invoice_number,
			TO_CHAR(i.invoice_date, 'DD Mon YYYY'),
			TO_CHAR(i.due_date, 'DD Mon YYYY'),
			i.subtotal,
			i.tax,
			i.total,
			COALESCE(i.discount, 0),
			i.paid_amount,
			i.remaining_amount,
			COALESCE(i.notes, ''),
			c.name
		FROM invoices i
		JOIN companies c ON c.id = i.company_id
		WHERE i.id = $1
	`, invoiceID).Scan(
		&data.Invoice.InvoiceNumber,
		&data.Invoice.InvoiceDate,
		&data.Invoice.DueDate,
		&data.Invoice.Subtotal,
		&data.Invoice.Tax,
		&data.Invoice.Total,
		&data.Invoice.Discount,
		&data.Invoice.AmountPaid,
		&data.Invoice.AmountDue,
		&data.Invoice.Notes,
		&data.Company.Name,
	)

	if err != nil {
		return data, fmt.Errorf("fetch invoice: %w", err)
	}

	/* -----------------------------
	   2️⃣ Fetch company address
	------------------------------ */
	err = db.QueryRow(`
    SELECT
        COALESCE(c.name, ''),
        COALESCE(c.address, ''),
        COALESCE(c.city, ''),
        COALESCE(c.state, ''),
        'India'
    FROM companies c
    JOIN invoices i ON i.company_id = c.id
    WHERE i.id = $1
`, invoiceID).Scan(
		&data.CompanyAddress.Name,
		&data.CompanyAddress.Line1,
		&data.CompanyAddress.City,
		&data.CompanyAddress.State,
		&data.CompanyAddress.Country,
	)

	if err != nil {
		return data, fmt.Errorf("fetch company address: %w", err)
	}

	/* -----------------------------
	   3️⃣ Fetch invoice addresses
	------------------------------ */
	rows, err := db.Query(`
		SELECT
			type,
			COALESCE(name, ''),
			COALESCE(line1, ''),
			COALESCE(city, ''),
			COALESCE(state, ''),
			COALESCE(country, '')
		FROM invoice_addresses
		WHERE invoice_id = $1
	`, invoiceID)

	if err != nil {
		return data, fmt.Errorf("fetch invoice addresses: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var addrType string
		var addr pdf.Address

		if err := rows.Scan(
			&addrType,
			&addr.Name,
			&addr.Line1,
			&addr.City,
			&addr.State,
			&addr.Country,
		); err != nil {
			return data, err
		}

		if addrType == "billing" {
			data.ClientBilling = addr
		} else if addrType == "shipping" {
			data.ClientShipping = &addr
		}
	}

	/* -----------------------------
	   4️⃣ Fetch invoice items
	------------------------------ */
	itemRows, err := db.Query(`
		SELECT
			it.name,
			COALESCE(it.hsn_code, ''),
			ii.qty,
			ii.rate,
			COALESCE(ii.tax_rate, 0),
			ii.total
		FROM invoice_items ii
		JOIN items it ON it.id = ii.item_id
		WHERE ii.invoice_id = $1
		ORDER BY ii.id
	`, invoiceID)

	if err != nil {
		return data, fmt.Errorf("fetch items: %w", err)
	}
	defer itemRows.Close()

	// invoice_items.total is stored tax-inclusive, so the taxable value has to be
	// backed out of it before the tax box can show a rate-wise breakdown.
	taxByRate := map[float64]*pdf.TaxLine{}
	rateOrder := []float64{}

	for itemRows.Next() {
		var item pdf.InvoiceItem

		if err := itemRows.Scan(
			&item.Name,
			&item.HSNCode,
			&item.Qty,
			&item.Rate,
			&item.TaxRate,
			&item.Total,
		); err != nil {
			return data, err
		}

		taxable := item.Total
		if item.TaxRate > 0 {
			taxable = item.Total / (1 + item.TaxRate/100)
		}
		item.Taxable = taxable

		line, seen := taxByRate[item.TaxRate]
		if !seen {
			line = &pdf.TaxLine{Rate: item.TaxRate}
			taxByRate[item.TaxRate] = line
			rateOrder = append(rateOrder, item.TaxRate)
		}
		line.Taxable += taxable
		line.Amount += item.Total - taxable

		data.Items = append(data.Items, item)
	}

	for _, rate := range rateOrder {
		data.Invoice.TaxLines = append(data.Invoice.TaxLines, *taxByRate[rate])
	}

	// Same place-of-supply rule the GST report uses, so a filed return and the printed
	// invoice can never disagree. An unrecorded place of supply (walk-in cash sale, or
	// an invoice predating address snapshots) falls back to the supplier's own state
	// under GST — it must not silently become an interstate IGST sale.
	companyState := strings.TrimSpace(data.CompanyAddress.State)
	billingState := strings.TrimSpace(data.ClientBilling.State)
	data.Invoice.IsInterstate = companyState != "" && billingState != "" &&
		!strings.EqualFold(companyState, billingState)

	/* -----------------------------
	   5️⃣ Fetch Default Bank Details
	------------------------------ */
	err = db.QueryRow(`
        SELECT 
            bank_name, 
            account_number, 
            ifsc_code, 
            COALESCE(branch, '')
        FROM company_bank_accounts
        WHERE company_id = (
            SELECT company_id FROM invoices WHERE id = $1
        )
        AND is_default = true
        LIMIT 1
    `, invoiceID).Scan(
		&data.Bank.BankName,
		&data.Bank.AccountNumber,
		&data.Bank.IFSCCode,
		&data.Bank.Branch,
	)

	// Optional: If no default bank is found, we can either return an error
	// or just leave it blank. Here we handle the "No Row" case gracefully.
	if err != nil && err != sql.ErrNoRows {
		return data, fmt.Errorf("fetch bank details: %w", err)
	}

	return data, nil
}
