ALTER TABLE items
    ADD COLUMN category INT,
    DROP COLUMN IF EXISTS category_id,
    DROP COLUMN IF EXISTS unit;
