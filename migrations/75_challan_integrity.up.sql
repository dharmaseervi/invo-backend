-- Composite keys enforce tenant identity even if a future write path omits validation.
-- NOT VALID permits an upgrade with historical bad rows without exposing them; the
-- handlers exclude foreign joins and block dispatch until those rows are corrected.
ALTER TABLE clients ADD CONSTRAINT clients_id_company_key UNIQUE (id, company_id);
ALTER TABLE items ADD CONSTRAINT items_id_company_key UNIQUE (id, company_id);
ALTER TABLE invoices ADD CONSTRAINT invoices_id_company_client_key UNIQUE (id, company_id, client_id);
ALTER TABLE delivery_challans ADD CONSTRAINT challans_id_company_key UNIQUE (id, company_id);
ALTER TABLE delivery_challans ADD CONSTRAINT challan_client_company_fk FOREIGN KEY (client_id, company_id) REFERENCES clients(id,company_id) NOT VALID;
ALTER TABLE delivery_challans ADD CONSTRAINT challan_invoice_company_client_fk FOREIGN KEY (invoice_id, company_id, client_id) REFERENCES invoices(id,company_id,client_id) NOT VALID;
ALTER TABLE delivery_challan_items ADD COLUMN company_id INTEGER;
UPDATE delivery_challan_items i SET company_id=c.company_id FROM delivery_challans c WHERE c.id=i.challan_id;
ALTER TABLE delivery_challan_items ALTER COLUMN company_id SET NOT NULL;
ALTER TABLE delivery_challan_items ADD CONSTRAINT challan_item_parent_company_fk FOREIGN KEY (challan_id,company_id) REFERENCES delivery_challans(id,company_id) ON DELETE CASCADE NOT VALID;
ALTER TABLE delivery_challan_items ADD CONSTRAINT challan_item_company_fk FOREIGN KEY (item_id,company_id) REFERENCES items(id,company_id) NOT VALID;
ALTER TABLE delivery_challan_items ADD CONSTRAINT challan_positive_qty CHECK (qty>0 AND qty<=1000000000) NOT VALID;
ALTER TABLE delivery_challans ADD CONSTRAINT challan_status CHECK (status IN ('draft','dispatched')) NOT VALID;
CREATE INDEX delivery_challans_company_date_idx ON delivery_challans(company_id,created_at DESC,id DESC);
CREATE INDEX delivery_challan_items_parent_idx ON delivery_challan_items(challan_id);
