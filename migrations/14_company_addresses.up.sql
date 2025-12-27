CREATE TABLE company_addresses (
    id SERIAL PRIMARY KEY,
    company_id INT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    type VARCHAR(20) CHECK (type IN ('billing', 'shipping')),
    name VARCHAR(255),
    line1 TEXT NOT NULL,
    line2 TEXT,
    city VARCHAR(100),
    state VARCHAR(100),
    postal_code VARCHAR(20),
    country VARCHAR(100),
    gst_number VARCHAR(50),
    phone VARCHAR(30),
    is_default BOOLEAN DEFAULT false,
    created_at TIMESTAMP DEFAULT NOW()
);
