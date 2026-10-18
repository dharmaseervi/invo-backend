DROP INDEX delivery_challan_items_parent_idx;
DROP INDEX delivery_challans_company_date_idx;
ALTER TABLE delivery_challan_items DROP CONSTRAINT challan_item_company_fk, DROP CONSTRAINT challan_item_parent_company_fk, DROP CONSTRAINT challan_positive_qty, DROP COLUMN company_id;
ALTER TABLE delivery_challans DROP CONSTRAINT challan_client_company_fk, DROP CONSTRAINT challan_invoice_company_client_fk, DROP CONSTRAINT challan_status, DROP CONSTRAINT challans_id_company_key;
ALTER TABLE invoices DROP CONSTRAINT invoices_id_company_client_key;
ALTER TABLE items DROP CONSTRAINT items_id_company_key;
ALTER TABLE clients DROP CONSTRAINT clients_id_company_key;
