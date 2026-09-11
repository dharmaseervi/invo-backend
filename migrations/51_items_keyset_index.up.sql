-- The item list is ordered by name and paged with a (name, id) keyset cursor. Without
-- an index in that exact order Postgres sorts the whole tenant's catalogue on every
-- page request, which is the opposite of what paging is for.
CREATE INDEX IF NOT EXISTS idx_items_company_name_id ON items(company_id, name, id);
