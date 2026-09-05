-- The original invoice_id had no default and no way to recover which invoice each
-- existing payment belonged to, so this can only restore the column shape, not the data.
ALTER TABLE payments ADD COLUMN IF NOT EXISTS invoice_id BIGINT;
