ALTER TABLE invoices
    DROP COLUMN IF EXISTS status,
    DROP COLUMN IF EXISTS paid_amount,
    DROP COLUMN IF EXISTS remaining_amount,
    DROP COLUMN IF EXISTS notes;
