package services

import (
	"database/sql"
	"fmt"

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
			i.invoice_date,
			i.due_date,
			i.subtotal,
			i.tax,
			i.total,
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
			name, line1, city, state, country
		FROM company_addresses
		WHERE company_id = (
			SELECT company_id FROM invoices WHERE id = $1
		)
		AND type = 'billing'
		LIMIT 1
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
			type, name, line1, city, state, country
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
			ii.qty,
			ii.rate,
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

	for itemRows.Next() {
		var item pdf.InvoiceItem

		if err := itemRows.Scan(
			&item.Name,
			&item.Qty,
			&item.Rate,
			&item.Total,
		); err != nil {
			return data, err
		}

		data.Items = append(data.Items, item)
	}

	return data, nil
}
