-- 1. Essential constraints and indexes
ALTER TABLE payment_allocations 
ADD CONSTRAINT unique_payment_invoice 
UNIQUE (payment_id, invoice_id);

CREATE INDEX idx_payment_allocations_payment ON payment_allocations(payment_id);
CREATE INDEX idx_payment_allocations_invoice ON payment_allocations(invoice_id);

-- 2. Add outstanding_balance to clients (if not exists)
ALTER TABLE clients 
ADD COLUMN IF NOT EXISTS outstanding_balance NUMERIC(12,2) DEFAULT 0;

-- 3. Add updated_at to payments for audit trail
ALTER TABLE payments 
ADD COLUMN updated_at TIMESTAMP DEFAULT NOW();

CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ language 'plpgsql';

CREATE TRIGGER update_payments_updated_at 
BEFORE UPDATE ON payments 
FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();