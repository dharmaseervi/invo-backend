-- Audit trail for every stock change: manual edits, restocks, and sales (invoice issue).
-- Without this, overwriting an item's quantity leaves no record of why it changed.
CREATE TABLE stock_movements (
    id SERIAL PRIMARY KEY,
    item_id INT NOT NULL REFERENCES items(id) ON DELETE CASCADE,
    company_id INT NOT NULL,
    user_id INT NOT NULL,
    movement_type VARCHAR(20) NOT NULL CHECK (movement_type IN ('restock', 'sale', 'adjustment', 'initial')),
    quantity_change INT NOT NULL,
    previous_quantity INT NOT NULL,
    new_quantity INT NOT NULL,
    reference VARCHAR(255),
    note TEXT,
    created_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX idx_stock_movements_item ON stock_movements(item_id);
CREATE INDEX idx_stock_movements_company ON stock_movements(company_id);
