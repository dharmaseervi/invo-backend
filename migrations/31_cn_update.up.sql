UPDATE credit_notes
SET type = CASE
    WHEN type = 'item' THEN 'return'
    WHEN type = 'value' THEN 'adjustment'
    WHEN type IS NULL THEN 'adjustment'
    ELSE 'adjustment'
END;
