package services

import (
	"database/sql"
	"fmt"

	"invo-server/internal/pdf"
)

// FetchEstimatePDFData builds the same InvoicePDFData shape the invoice PDF generators
// already render, from an estimate instead. Estimates don't snapshot a billing/shipping
// address the way invoices do (a quotation isn't a legal tax document), so this falls
// back to the client's saved billing address, then the plain client record.
func FetchEstimatePDFData(db *sql.DB, estimateID int) (pdf.InvoicePDFData, error) {
	var data pdf.InvoicePDFData
	data.Invoice.DocType = "QUOTATION"

	var clientID int
	err := db.QueryRow(`
		SELECT
			e.estimate_number,
			e.estimate_date,
			COALESCE(e.expiry_date::text, ''),
			e.subtotal,
			e.tax,
			e.discount,
			e.total,
			e.client_id,
			c.name
		FROM estimates e
		JOIN companies c ON c.id = e.company_id
		WHERE e.id = $1
	`, estimateID).Scan(
		&data.Invoice.InvoiceNumber,
		&data.Invoice.InvoiceDate,
		&data.Invoice.DueDate,
		&data.Invoice.Subtotal,
		&data.Invoice.Tax,
		&data.Invoice.Discount,
		&data.Invoice.Total,
		&clientID,
		&data.Company.Name,
	)
	if err != nil {
		return data, fmt.Errorf("fetch estimate: %w", err)
	}
	data.Invoice.AmountDue = data.Invoice.Total

	err = db.QueryRow(`
		SELECT
			COALESCE(c.name, ''),
			COALESCE(c.address, ''),
			COALESCE(c.city, ''),
			COALESCE(c.state, ''),
			'India'
		FROM companies c
		JOIN estimates e ON e.company_id = c.id
		WHERE e.id = $1
	`, estimateID).Scan(
		&data.CompanyAddress.Name,
		&data.CompanyAddress.Line1,
		&data.CompanyAddress.City,
		&data.CompanyAddress.State,
		&data.CompanyAddress.Country,
	)
	if err != nil {
		return data, fmt.Errorf("fetch company address: %w", err)
	}

	// Client billing address: saved address first, plain client record as fallback.
	err = db.QueryRow(`
		SELECT COALESCE(name, ''), COALESCE(line1, ''), COALESCE(city, ''), COALESCE(state, ''), COALESCE(country, '')
		FROM client_addresses
		WHERE client_id = $1 AND type = 'billing'
		LIMIT 1
	`, clientID).Scan(
		&data.ClientBilling.Name, &data.ClientBilling.Line1,
		&data.ClientBilling.City, &data.ClientBilling.State, &data.ClientBilling.Country,
	)
	if err == sql.ErrNoRows {
		err = db.QueryRow(`
			SELECT COALESCE(name, ''), COALESCE(address, ''), COALESCE(city, ''), COALESCE(state, ''), 'India'
			FROM clients WHERE id = $1
		`, clientID).Scan(
			&data.ClientBilling.Name, &data.ClientBilling.Line1,
			&data.ClientBilling.City, &data.ClientBilling.State, &data.ClientBilling.Country,
		)
	}
	if err != nil && err != sql.ErrNoRows {
		return data, fmt.Errorf("fetch client address: %w", err)
	}

	itemRows, err := db.Query(`
		SELECT it.name, COALESCE(it.hsn_code, ''), ei.qty, ei.rate, ei.total
		FROM estimate_items ei
		JOIN items it ON it.id = ei.item_id
		WHERE ei.estimate_id = $1
		ORDER BY ei.id
	`, estimateID)
	if err != nil {
		return data, fmt.Errorf("fetch estimate items: %w", err)
	}
	defer itemRows.Close()

	for itemRows.Next() {
		var item pdf.InvoiceItem
		if err := itemRows.Scan(&item.Name, &item.HSNCode, &item.Qty, &item.Rate, &item.Total); err != nil {
			return data, err
		}
		data.Items = append(data.Items, item)
	}

	err = db.QueryRow(`
		SELECT bank_name, account_number, ifsc_code, COALESCE(branch, '')
		FROM company_bank_accounts
		WHERE company_id = (SELECT company_id FROM estimates WHERE id = $1)
		AND is_default = true
		LIMIT 1
	`, estimateID).Scan(&data.Bank.BankName, &data.Bank.AccountNumber, &data.Bank.IFSCCode, &data.Bank.Branch)
	if err != nil && err != sql.ErrNoRows {
		return data, fmt.Errorf("fetch bank details: %w", err)
	}

	return data, nil
}
