-- Nothing currently stops two items from sharing a SKU, which also breaks the
-- scan-to-lookup feature (it matches by SKU — a duplicate makes the match ambiguous).
-- Scoped per-company and skipping blanks, same pattern as the earlier clients.email fix.
CREATE UNIQUE INDEX items_company_id_sku_unique
    ON items (company_id, sku)
    WHERE sku <> '';
