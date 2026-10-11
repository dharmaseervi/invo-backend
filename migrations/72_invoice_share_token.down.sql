DROP INDEX IF EXISTS idx_invoices_share_token;
ALTER TABLE invoices DROP COLUMN IF EXISTS share_token;
