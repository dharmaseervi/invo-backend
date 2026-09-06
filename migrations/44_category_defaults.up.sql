-- Lets a category carry a default HSN code + GST rate (e.g. "Enamel Paints" -> HSN 3208, 18%)
-- so new items created under it can inherit these instead of retyping them every time.
ALTER TABLE categories ADD COLUMN default_hsn_code VARCHAR(20);
ALTER TABLE categories ADD COLUMN default_tax_rate NUMERIC(5,2);
