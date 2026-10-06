ALTER TABLE day_closings
    DROP COLUMN IF EXISTS in_breakdown,
    DROP COLUMN IF EXISTS out_breakdown;
