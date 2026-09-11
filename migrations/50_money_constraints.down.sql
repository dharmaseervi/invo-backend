ALTER TABLE invoices DROP CONSTRAINT IF EXISTS invoices_amounts_non_negative;
ALTER TABLE invoice_items DROP CONSTRAINT IF EXISTS invoice_items_amounts_non_negative;
ALTER TABLE estimates DROP CONSTRAINT IF EXISTS estimates_amounts_non_negative;
ALTER TABLE estimate_items DROP CONSTRAINT IF EXISTS estimate_items_amounts_non_negative;
ALTER TABLE payments DROP CONSTRAINT IF EXISTS payments_amount_positive;
ALTER TABLE payment_allocations DROP CONSTRAINT IF EXISTS payment_allocations_amount_positive;
ALTER TABLE ledger_entries DROP CONSTRAINT IF EXISTS ledger_entries_amounts_non_negative;
ALTER TABLE items DROP CONSTRAINT IF EXISTS items_prices_non_negative;
