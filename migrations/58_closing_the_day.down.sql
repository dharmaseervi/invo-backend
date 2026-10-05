ALTER TABLE stock_movements DROP CONSTRAINT IF EXISTS stock_movements_movement_type_check;
ALTER TABLE stock_movements ADD CONSTRAINT stock_movements_movement_type_check
    CHECK (movement_type IN (
        'restock', 'sale', 'adjustment', 'initial', 'purchase', 'purchase_return'
    ));

DROP TABLE IF EXISTS day_closings;
DROP TABLE IF EXISTS stocktake_lines;
DROP TABLE IF EXISTS stocktakes;
