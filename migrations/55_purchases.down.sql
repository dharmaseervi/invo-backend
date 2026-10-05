DROP TABLE IF EXISTS supplier_payments;
DROP TABLE IF EXISTS purchase_bill_items;
DROP TABLE IF EXISTS purchase_bills;
DROP TABLE IF EXISTS suppliers;

ALTER TABLE stock_movements
    DROP CONSTRAINT IF EXISTS stock_movements_movement_type_check;
ALTER TABLE stock_movements
    ADD CONSTRAINT stock_movements_movement_type_check
    CHECK (movement_type IN ('restock', 'sale', 'adjustment', 'initial'));
