DROP TABLE IF EXISTS refunds;
ALTER TABLE payments
    DROP CONSTRAINT IF EXISTS payments_status_check,
    DROP COLUMN IF EXISTS status,
    DROP COLUMN IF EXISTS reversed_at,
    DROP COLUMN IF EXISTS reversal_reason,
    DROP COLUMN IF EXISTS unapplied_amount;

ALTER TABLE ledger_entries
    DROP CONSTRAINT IF EXISTS ledger_entries_source_type_check;
ALTER TABLE ledger_entries
    ADD CONSTRAINT ledger_entries_source_type_check
    CHECK (source_type IN ('INVOICE', 'PAYMENT', 'CREDIT_NOTE', 'ADJUSTMENT'));
