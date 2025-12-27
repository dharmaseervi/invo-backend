CREATE TABLE client_addresses (
    id SERIAL PRIMARY KEY,
    client_id INT NOT NULL REFERENCES clients(id) ON DELETE CASCADE,

    type VARCHAR(20) NOT NULL CHECK (type IN ('billing', 'shipping')),

    name VARCHAR(255), -- Business / Person name
    line1 TEXT NOT NULL,
    line2 TEXT,
    city VARCHAR(100),
    state VARCHAR(100),
    postal_code VARCHAR(20),
    country VARCHAR(100),

    phone VARCHAR(30),
    email VARCHAR(255),
    gst_number VARCHAR(50),

    is_default BOOLEAN DEFAULT false,

    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);
