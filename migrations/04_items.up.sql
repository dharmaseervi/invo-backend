CREATE TABLE items (
    id SERIAL PRIMARY KEY,
    company_id INTEGER NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    category_id INTEGER REFERENCES categories(id) ON DELETE SET NULL,
    unit VARCHAR(50),
    sku VARCHAR(255),
    description TEXT,
    cost_price NUMERIC(10,2),
    price NUMERIC(10,2) NOT NULL,
    quantity INT DEFAULT 0,
    low_stock_alert INT DEFAULT 0,
    tax_rate NUMERIC(5,2),
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);
