-- The Cash and UPI accounts a shop bills walk-in sales to.
--
-- These are ordinary clients named "Cash" and "UPI", so invoices, payments and the
-- per-customer ledger all work on them unmodified. The app found them by fetching every
-- client and looking through the list — which meant the client list could never be
-- paged, because the one it needed might be on page four.
--
-- Two things were wrong with that beyond the paging. Nothing stopped a second "Cash"
-- being created when two tills asked at once, and the app would then bill to whichever
-- one happened to come back first.

-- Any shop that already has two accounts by the same name keeps both, with the later
-- ones renamed rather than merged. Merging would move invoices between customers, which
-- is not a thing a migration should decide to do quietly; renaming leaves every invoice
-- where it is and makes the duplicate visible to whoever has to sort it out.
WITH ranked AS (
    SELECT id,
           name,
           ROW_NUMBER() OVER (
               PARTITION BY company_id, lower(name) ORDER BY id
           ) AS copy
    FROM clients
    WHERE lower(name) IN ('cash', 'upi')
)
UPDATE clients c
SET name = ranked.name || ' (' || ranked.copy || ')'
FROM ranked
WHERE c.id = ranked.id AND ranked.copy > 1;

-- One per company from here on. Case-insensitive, because somebody typing "cash" means
-- the same account.
CREATE UNIQUE INDEX IF NOT EXISTS clients_one_quick_sale_account_per_company
    ON clients (company_id, lower(name))
    WHERE lower(name) IN ('cash', 'upi');
