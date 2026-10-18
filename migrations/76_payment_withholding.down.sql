-- A withholding-only settlement cannot be represented by the old schema. Refuse destructive rollback.
DO $$ BEGIN
 IF EXISTS (SELECT 1 FROM payments WHERE tds_amount > 0) THEN
  RAISE EXCEPTION 'Cannot remove withholding schema while withholding settlements exist';
 END IF;
END $$;
ALTER TABLE payments DROP CONSTRAINT payments_settlement_positive;
ALTER TABLE payments ADD CONSTRAINT payments_amount_positive CHECK (amount > 0) NOT VALID;
ALTER TABLE payment_allocations DROP COLUMN tds_amount;
ALTER TABLE payments DROP COLUMN tds_amount;
