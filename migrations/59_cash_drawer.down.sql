DROP INDEX IF EXISTS expensess_company_method_date_idx;
ALTER TABLE expensess DROP COLUMN IF EXISTS payment_method;
ALTER TABLE day_closings
    DROP COLUMN IF EXISTS opening_cash,
    DROP COLUMN IF EXISTS cash_in,
    DROP COLUMN IF EXISTS cash_out;
