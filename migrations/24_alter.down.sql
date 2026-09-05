DROP TRIGGER IF EXISTS update_payments_updated_at ON payments;
DROP FUNCTION IF EXISTS update_updated_at_column();

ALTER TABLE payments DROP COLUMN IF EXISTS updated_at;
ALTER TABLE clients DROP COLUMN IF EXISTS outstanding_balance;

DROP INDEX IF EXISTS idx_payment_allocations_invoice;
DROP INDEX IF EXISTS idx_payment_allocations_payment;
ALTER TABLE payment_allocations DROP CONSTRAINT IF EXISTS unique_payment_invoice;
