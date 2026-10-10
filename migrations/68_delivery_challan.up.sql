CREATE TABLE IF NOT EXISTS delivery_challans (
    id              SERIAL PRIMARY KEY,
    company_id      INTEGER NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    client_id       INTEGER NOT NULL REFERENCES clients(id),
    challan_number  VARCHAR(50) NOT NULL,
    challan_date    DATE NOT NULL,
    status          VARCHAR(20) NOT NULL DEFAULT 'draft',
    invoice_id      INTEGER REFERENCES invoices(id),
    notes           TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE TABLE IF NOT EXISTS delivery_challan_items (
    id          SERIAL PRIMARY KEY,
    challan_id  INTEGER NOT NULL REFERENCES delivery_challans(id) ON DELETE CASCADE,
    item_id     INTEGER NOT NULL REFERENCES items(id),
    qty         INTEGER NOT NULL DEFAULT 1,
    description TEXT NOT NULL DEFAULT ''
);
CREATE SEQUENCE IF NOT EXISTS delivery_challan_seq START 1;
