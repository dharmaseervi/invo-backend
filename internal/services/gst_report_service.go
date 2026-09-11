package services

import (
	"database/sql"
	"strconv"
	"strings"

	"invo-server/internal/models"
)

// GenerateGSTReport builds a GSTR-1-style summary (invoice-wise + HSN-wise) for a company
// over a date range. Every invoice_items.total is stored tax-inclusive, so the taxable
// value and tax amount are derived from it and the line's tax_rate. CGST/SGST vs IGST is
// decided by comparing the company's registered state against the invoice's billing state
// (the place of supply) — same state means intrastate (CGST+SGST split), otherwise
// interstate (IGST).
func GenerateGSTReport(db *sql.DB, companyID int64, start, end string) (*models.GSTReportResponse, error) {

	var companyState string
	if err := db.QueryRow(`SELECT COALESCE(state, '') FROM companies WHERE id = $1`, companyID).Scan(&companyState); err != nil {
		return nil, err
	}

	rows, err := db.Query(`
		SELECT
			i.id,
			i.invoice_number,
			i.invoice_date,
			c.name,
			COALESCE(ia.gst_number, ''),
			COALESCE(ia.state, ''),
			COALESCE(it.hsn_code, ''),
			ii.qty,
			ii.tax_rate,
			ii.total
		FROM invoices i
		JOIN clients c ON c.id = i.client_id
		JOIN invoice_items ii ON ii.invoice_id = i.id
		LEFT JOIN items it ON it.id = ii.item_id
		LEFT JOIN invoice_addresses ia ON ia.invoice_id = i.id AND ia.type = 'billing'
		WHERE i.company_id = $1
		  AND i.invoice_date BETWEEN $2 AND $3
		  AND i.status NOT IN ('draft', 'cancelled')
		ORDER BY i.invoice_date, i.id
	`, companyID, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	invoiceOrder := []int{}
	invoiceByID := map[int]*models.GSTInvoiceSummaryRow{}
	hsnKeyOrder := []string{}
	hsnByKey := map[string]*models.GSTHSNSummaryRow{}
	summary := models.GSTSummary{}

	for rows.Next() {
		var (
			invoiceID                    int
			invoiceNumber, invoiceDate   string
			clientName, clientGSTIN      string
			billingState, hsnCode        string
			qty, taxRate, lineTotalIncTx float64
		)
		if err := rows.Scan(
			&invoiceID, &invoiceNumber, &invoiceDate, &clientName,
			&clientGSTIN, &billingState, &hsnCode, &qty, &taxRate, &lineTotalIncTx,
		); err != nil {
			return nil, err
		}

		taxableValue := lineTotalIncTx
		if taxRate > 0 {
			taxableValue = lineTotalIncTx / (1 + taxRate/100)
		}
		taxAmount := lineTotalIncTx - taxableValue

		// An unrecorded place of supply (walk-in cash sale, or an invoice created before
		// addresses were snapshotted) defaults to the supplier's own state under GST —
		// treating it as interstate would report local sales as IGST in the return.
		trimmedCompany := strings.TrimSpace(companyState)
		trimmedBilling := strings.TrimSpace(billingState)
		intrastate := trimmedCompany == "" || trimmedBilling == "" ||
			strings.EqualFold(trimmedCompany, trimmedBilling)

		var cgst, sgst, igst float64
		if intrastate {
			cgst = taxAmount / 2
			sgst = taxAmount / 2
		} else {
			igst = taxAmount
		}

		inv, exists := invoiceByID[invoiceID]
		if !exists {
			inv = &models.GSTInvoiceSummaryRow{
				InvoiceID:     invoiceID,
				InvoiceNumber: invoiceNumber,
				InvoiceDate:   invoiceDate,
				ClientName:    clientName,
				ClientGSTIN:   clientGSTIN,
				PlaceOfSupply: billingState,
			}
			invoiceByID[invoiceID] = inv
			invoiceOrder = append(invoiceOrder, invoiceID)
			summary.InvoiceCount++
		}
		inv.TaxableValue += taxableValue
		inv.CGST += cgst
		inv.SGST += sgst
		inv.IGST += igst
		inv.Total += lineTotalIncTx

		hsnKey := hsnCode + "|" + formatRate(taxRate)
		hsn, exists := hsnByKey[hsnKey]
		if !exists {
			hsn = &models.GSTHSNSummaryRow{HSNCode: hsnCode, TaxRate: taxRate}
			hsnByKey[hsnKey] = hsn
			hsnKeyOrder = append(hsnKeyOrder, hsnKey)
		}
		hsn.TotalQty += qty
		hsn.TaxableValue += taxableValue
		hsn.CGST += cgst
		hsn.SGST += sgst
		hsn.IGST += igst
		hsn.TotalValue += lineTotalIncTx

		summary.TaxableValue += taxableValue
		summary.CGST += cgst
		summary.SGST += sgst
		summary.IGST += igst
		summary.Total += lineTotalIncTx
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	invoices := make([]models.GSTInvoiceSummaryRow, 0, len(invoiceOrder))
	for _, id := range invoiceOrder {
		invoices = append(invoices, *invoiceByID[id])
	}

	hsnSummary := make([]models.GSTHSNSummaryRow, 0, len(hsnKeyOrder))
	for _, key := range hsnKeyOrder {
		hsnSummary = append(hsnSummary, *hsnByKey[key])
	}

	return &models.GSTReportResponse{
		Start:        start,
		End:          end,
		CompanyState: companyState,
		Summary:      summary,
		Invoices:     invoices,
		HSNSummary:   hsnSummary,
	}, nil
}

func formatRate(rate float64) string {
	return strconv.FormatFloat(rate, 'f', -1, 64)
}
