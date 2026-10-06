-- Keep the explanation of a closing with its totals. NULL means this closing predates
-- breakdown snapshots; the API uses its saved aggregate totals for those older rows.
ALTER TABLE day_closings
    ADD COLUMN IF NOT EXISTS in_breakdown JSONB,
    ADD COLUMN IF NOT EXISTS out_breakdown JSONB;
