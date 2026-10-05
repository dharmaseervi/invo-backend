-- "Which businesses does this person work in?", asked the same way everywhere.
--
-- Records carry the id of whoever created them, and the queries that read them back
-- filtered on it: WHERE i.user_id = $1. While a shop was one account that was the same
-- thing as "this shop's invoices". With staff it stops being the same thing, in both
-- directions — the counter boy would see an empty list, and the owner would stop seeing
-- the invoices his staff wrote.
--
-- So the filter becomes the company, and the company is this. Kept as a function rather
-- than spelled out forty times: it has to mean exactly one thing in every place that
-- asks, and when it changes it has to change everywhere at once.
--
-- STABLE, so the planner may call it once per statement rather than once per row.

CREATE OR REPLACE FUNCTION companies_for_user(uid INTEGER)
RETURNS TABLE (company_id BIGINT)
LANGUAGE sql
STABLE
AS $$
    -- The owner's own businesses, and anywhere they have been given a login. The owner
    -- row is read as well as membership so that a company somehow saved without its
    -- member row never locks out the person it belongs to.
    SELECT c.id::BIGINT FROM companies c WHERE c.user_id = uid
    UNION
    SELECT m.company_id FROM company_members m WHERE m.user_id = uid
$$;
