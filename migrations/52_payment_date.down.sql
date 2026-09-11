DROP INDEX IF EXISTS idx_payments_company_date;
ALTER TABLE payments DROP COLUMN IF EXISTS payment_date;
