-- Money actually received stays in amount; withholding is an independently auditable settlement.
ALTER TABLE payments ADD COLUMN tds_amount NUMERIC(12,2) NOT NULL DEFAULT 0 CHECK (tds_amount >= 0);
ALTER TABLE payment_allocations ADD COLUMN tds_amount NUMERIC(12,2) NOT NULL DEFAULT 0 CHECK (tds_amount >= 0 AND tds_amount <= amount);
ALTER TABLE payments DROP CONSTRAINT IF EXISTS payments_amount_positive;
ALTER TABLE payments ADD CONSTRAINT payments_settlement_positive CHECK (amount >= 0 AND amount + tds_amount > 0) NOT VALID;
