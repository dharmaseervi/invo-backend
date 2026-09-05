ALTER TABLE invoices
    DROP COLUMN IF EXISTS pdf_url,
    DROP COLUMN IF EXISTS pdf_generated_at;
