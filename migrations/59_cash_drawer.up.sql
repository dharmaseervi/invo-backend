-- What is actually in the drawer, rather than only what came into it.
--
-- The first cut of cash closing compared the count against the day's cash *receipts*
-- alone. That is not what a till holds. It holds whatever was in it this morning, plus
-- what came in, less what went out of it — and a shop pays its suppliers, refunds
-- customers and buys tea out of the same drawer.
--
-- So every closing was wrong by the float, and wrong again by anything paid out. A shop
-- that starts each day with 2,000 and pays a supplier 5,000 in cash would have been told
-- it was 3,000 short, every single day, until somebody stopped believing the figure.

ALTER TABLE day_closings
    -- What was in the drawer before the day started. Carried over from the last
    -- closing's count by default, which is what actually happens to a till overnight.
    ADD COLUMN IF NOT EXISTS opening_cash NUMERIC(12,2) NOT NULL DEFAULT 0,
    -- The two sides, stored rather than derived, for the same reason expected_cash is:
    -- a closing records what was true when somebody counted, and a payment edited next
    -- week must not quietly change what last Tuesday appeared to find.
    ADD COLUMN IF NOT EXISTS cash_in  NUMERIC(12,2) NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS cash_out NUMERIC(12,2) NOT NULL DEFAULT 0;

-- Existing closings were taken on receipts alone, so that is what their cash_in was.
UPDATE day_closings SET cash_in = expected_cash WHERE cash_in = 0;

-- How an expense was paid.
--
-- Expenses had no method at all, so there was no way to tell rent paid by bank transfer
-- from a hundred rupees of tea taken out of the till. Left NULL on everything that
-- already exists: guessing "cash" for a year of history would move every past day's
-- drawer figure, and an unknown method is not the same as a known one.
ALTER TABLE expensess
    ADD COLUMN IF NOT EXISTS payment_method VARCHAR(50);

CREATE INDEX IF NOT EXISTS expensess_company_method_date_idx
    ON expensess (company_id, date, payment_method);
