-- Payments recorded only created_at, the moment the row was written. A shop entering
-- yesterday's cheque this morning had no way to say so, which puts the payment in the
-- wrong period for every report that groups by date.
ALTER TABLE payments ADD COLUMN IF NOT EXISTS payment_date DATE;

-- Existing rows keep the date they were entered, which is the best available answer.
UPDATE payments SET payment_date = created_at::date WHERE payment_date IS NULL;

ALTER TABLE payments ALTER COLUMN payment_date SET DEFAULT CURRENT_DATE;
ALTER TABLE payments ALTER COLUMN payment_date SET NOT NULL;

CREATE INDEX IF NOT EXISTS idx_payments_company_date ON payments(company_id, payment_date DESC);
