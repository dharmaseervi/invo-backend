-- Backfill missing state/GSTIN on invoice_addresses (type='billing') for invoices created
-- before the state field was a required dropdown. Needed so the GSTR-1 report's
-- intrastate/interstate (CGST+SGST vs IGST) split is correct for pre-existing invoices.
-- Priority: client's default billing address, then the client record's own state.

UPDATE invoice_addresses ia
SET state = ca.state
FROM invoices i
JOIN client_addresses ca ON ca.client_id = i.client_id AND ca.type = 'billing'
WHERE ia.invoice_id = i.id
  AND ia.type = 'billing'
  AND (ia.state IS NULL OR ia.state = '')
  AND ca.state IS NOT NULL AND ca.state != '';

UPDATE invoice_addresses ia
SET state = c.state
FROM invoices i
JOIN clients c ON c.id = i.client_id
WHERE ia.invoice_id = i.id
  AND ia.type = 'billing'
  AND (ia.state IS NULL OR ia.state = '')
  AND c.state IS NOT NULL AND c.state != '';

UPDATE invoice_addresses ia
SET gst_number = ca.gst_number
FROM invoices i
JOIN client_addresses ca ON ca.client_id = i.client_id AND ca.type = 'billing'
WHERE ia.invoice_id = i.id
  AND ia.type = 'billing'
  AND (ia.gst_number IS NULL OR ia.gst_number = '')
  AND ca.gst_number IS NOT NULL AND ca.gst_number != '';
