-- =========================
-- Invoices table
-- =========================
CREATE TABLE invoices (
    id SERIAL PRIMARY KEY,
    company_id INTEGER NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    client_id INTEGER NOT NULL REFERENCES clients(id) ON DELETE CASCADE,
    invoice_number VARCHAR(50),
    invoice_date DATE NOT NULL,
    due_date DATE NOT NULL,
    subtotal NUMERIC(10,2) NOT NULL,
    tax NUMERIC(10,2) NOT NULL,
    total NUMERIC(10,2) NOT NULL,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

-- =========================
-- Invoice Items table
-- =========================
CREATE TABLE invoice_items (
    id SERIAL PRIMARY KEY,
    invoice_id INTEGER NOT NULL REFERENCES invoices(id) ON DELETE CASCADE,
    item_id INTEGER REFERENCES items(id) ON DELETE SET NULL,
    qty INT NOT NULL CHECK (qty > 0),
    rate NUMERIC(10,2) NOT NULL CHECK (rate >= 0),
    discount NUMERIC(10,2) DEFAULT 0 CHECK (discount >= 0),
    tax_rate NUMERIC(5,2) DEFAULT 0 CHECK (tax_rate >= 0),

    total NUMERIC(10,2) NOT NULL
);
