CREATE TABLE estimates (
    id SERIAL PRIMARY KEY,
    company_id INTEGER NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    client_id INTEGER NOT NULL REFERENCES clients(id) ON DELETE CASCADE,

    estimate_number VARCHAR(50),
    estimate_date DATE NOT NULL,
    expiry_date DATE,

    subtotal NUMERIC(10,2) NOT NULL,
    tax NUMERIC(10,2) NOT NULL,
    discount NUMERIC(10,2) NOT NULL DEFAULT 0,
    total NUMERIC(10,2) NOT NULL,

    status VARCHAR(20) NOT NULL DEFAULT 'draft'
        CHECK (status IN ('draft', 'sent', 'accepted', 'rejected', 'expired', 'converted')),
    converted_invoice_id INTEGER REFERENCES invoices(id) ON DELETE SET NULL,

    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

CREATE TABLE estimate_items (
    id SERIAL PRIMARY KEY,
    estimate_id INTEGER NOT NULL REFERENCES estimates(id) ON DELETE CASCADE,
    item_id INTEGER REFERENCES items(id) ON DELETE SET NULL,
    qty INT NOT NULL CHECK (qty > 0),
    rate NUMERIC(10,2) NOT NULL CHECK (rate >= 0),
    discount NUMERIC(10,2) DEFAULT 0 CHECK (discount >= 0),
    tax_rate NUMERIC(5,2) DEFAULT 0 CHECK (tax_rate >= 0),
    total NUMERIC(10,2) NOT NULL
);

CREATE TABLE estimate_counters (
    company_id INTEGER NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    financial_year VARCHAR(10) NOT NULL,
    next_number INTEGER NOT NULL DEFAULT 1,
    PRIMARY KEY (company_id, financial_year)
);
