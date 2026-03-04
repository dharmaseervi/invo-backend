CREATE TABLE credit_notes (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id),
    client_id BIGINT NOT NULL REFERENCES clients(id),
    invoice_id BIGINT REFERENCES invoices(id),
    type TEXT NOT NULL CHECK (type IN ('item','value')),
    credit_number TEXT NOT NULL,
    credit_date DATE NOT NULL,
    reason TEXT,
    subtotal NUMERIC(12,2) DEFAULT 0,
    tax NUMERIC(12,2) DEFAULT 0,
    total NUMERIC(12,2) NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE credit_note_items (
    id BIGSERIAL PRIMARY KEY,
    credit_note_id BIGINT NOT NULL REFERENCES credit_notes(id) ON DELETE CASCADE,
    item_id BIGINT NOT NULL REFERENCES items(id),
    qty NUMERIC(10,2) NOT NULL,
    rate NUMERIC(12,2) NOT NULL,
    tax_rate NUMERIC(5,2) DEFAULT 0,
    total NUMERIC(12,2) NOT NULL
);

