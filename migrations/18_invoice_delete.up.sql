-- Remove the column
ALTER TABLE invoices DROP COLUMN pdf_url;
ALTER TABLE invoices DROP COLUMN pdf_generated_at;

-- Clean up any indexes
DROP INDEX IF EXISTS idx_invoices_pdf_url;