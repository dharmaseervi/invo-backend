ALTER TABLE expensess ADD COLUMN IF NOT EXISTS category VARCHAR(50);
CREATE INDEX IF NOT EXISTS idx_expensess_category ON expensess (company_id, category);
