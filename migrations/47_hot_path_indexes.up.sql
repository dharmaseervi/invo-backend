-- Postgres does not index foreign-key columns automatically, and almost every read in
-- this app filters by tenant (company_id / user_id) or by parent id. Without these,
-- each of those queries is a sequential scan that grows with total table size rather
-- than with the tenant's own data.

-- Tenant-scoped list reads
CREATE INDEX IF NOT EXISTS idx_items_company ON items(company_id);
CREATE INDEX IF NOT EXISTS idx_items_user ON items(user_id);
CREATE INDEX IF NOT EXISTS idx_clients_company ON clients(company_id);
CREATE INDEX IF NOT EXISTS idx_clients_user ON clients(user_id);
CREATE INDEX IF NOT EXISTS idx_categories_company ON categories(company_id);
CREATE INDEX IF NOT EXISTS idx_companies_user ON companies(user_id);

-- Invoice lists and the GST/aging reports filter by tenant then date.
CREATE INDEX IF NOT EXISTS idx_invoices_company_date ON invoices(company_id, invoice_date);
CREATE INDEX IF NOT EXISTS idx_invoices_user ON invoices(user_id);
-- Payment auto-allocation looks up a client's open invoices oldest-first.
CREATE INDEX IF NOT EXISTS idx_invoices_client_status ON invoices(client_id, status);
-- Aging report scans for anything still owed.
CREATE INDEX IF NOT EXISTS idx_invoices_outstanding ON invoices(company_id, due_date)
    WHERE remaining_amount > 0;

-- Parent-child joins done on every invoice read and PDF render.
CREATE INDEX IF NOT EXISTS idx_invoice_items_invoice ON invoice_items(invoice_id);
CREATE INDEX IF NOT EXISTS idx_invoice_items_item ON invoice_items(item_id);
CREATE INDEX IF NOT EXISTS idx_invoice_addresses_invoice ON invoice_addresses(invoice_id, type);
CREATE INDEX IF NOT EXISTS idx_client_addresses_client ON client_addresses(client_id, type);

-- Estimates mirror invoices.
CREATE INDEX IF NOT EXISTS idx_estimates_company_date ON estimates(company_id, estimate_date);
CREATE INDEX IF NOT EXISTS idx_estimates_user ON estimates(user_id);
CREATE INDEX IF NOT EXISTS idx_estimate_items_estimate ON estimate_items(estimate_id);

-- Expense list and date-range reporting. Table really is spelled "expensess" -
-- the typo is consistent across the schema and all queries, so it is left alone.
CREATE INDEX IF NOT EXISTS idx_expenses_company_date ON expensess(company_id, date);

-- Credit notes are looked up by the invoice they reverse.
CREATE INDEX IF NOT EXISTS idx_credit_notes_invoice ON credit_notes(invoice_id);
CREATE INDEX IF NOT EXISTS idx_credit_notes_company ON credit_notes(company_id);

-- The ledger is read as a per-client statement in insertion order.
CREATE INDEX IF NOT EXISTS idx_ledger_client_created ON ledger_entries(company_id, client_id, id);
