-- Best-effort only: 31's up collapsed both NULL and 'value' into 'adjustment',
-- so which rows were originally NULL vs 'value' can't be recovered.
UPDATE credit_notes
SET type = CASE
    WHEN type = 'return' THEN 'item'
    WHEN type = 'adjustment' THEN 'value'
    ELSE type
END;
