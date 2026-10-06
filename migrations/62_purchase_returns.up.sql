-- Sending stock back to a supplier.
--
-- Tiles arrive broken, or the wrong shade, or simply too many. The shop sends them back
-- and expects the bill to come down. Until now there was no way to record that: the
-- stock stayed on the books, the supplier stayed owed in full, and the only way to make
-- the figures right was to edit something until it looked right.
--
-- This is the buying-side twin of a credit note, and it is a debit note in the sense a
-- shopkeeper means: the supplier owes us this.

CREATE TABLE IF NOT EXISTS purchase_returns (
    id            BIGSERIAL PRIMARY KEY,
    company_id    BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    user_id       INTEGER NOT NULL,
    supplier_id   BIGINT NOT NULL REFERENCES suppliers(id) ON DELETE RESTRICT,
    -- The bill the goods came in on, where it is known. A shop that finds a broken box
    -- weeks later may not know which delivery it arrived on, and should still be able
    -- to send it back.
    bill_id       BIGINT REFERENCES purchase_bills(id) ON DELETE SET NULL,
    return_number VARCHAR(100) NOT NULL,
    return_date   DATE NOT NULL DEFAULT CURRENT_DATE,
    subtotal      NUMERIC(12,2) NOT NULL DEFAULT 0,
    tax           NUMERIC(12,2) NOT NULL DEFAULT 0,
    total         NUMERIC(12,2) NOT NULL DEFAULT 0,
    reason        TEXT,
    notes         TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT purchase_returns_number_per_supplier UNIQUE (supplier_id, return_number)
);

CREATE INDEX IF NOT EXISTS purchase_returns_company_date_idx
    ON purchase_returns (company_id, return_date DESC, id DESC);
CREATE INDEX IF NOT EXISTS purchase_returns_supplier_idx ON purchase_returns (supplier_id);
CREATE INDEX IF NOT EXISTS purchase_returns_bill_idx ON purchase_returns (bill_id);

CREATE TABLE IF NOT EXISTS purchase_return_items (
    id        BIGSERIAL PRIMARY KEY,
    return_id BIGINT NOT NULL REFERENCES purchase_returns(id) ON DELETE CASCADE,
    item_id   BIGINT NOT NULL REFERENCES items(id) ON DELETE RESTRICT,
    qty       INTEGER NOT NULL CHECK (qty > 0),
    rate      NUMERIC(12,2) NOT NULL CHECK (rate >= 0),
    tax_rate  NUMERIC(5,2) NOT NULL DEFAULT 0,
    total     NUMERIC(12,2) NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS purchase_return_items_return_idx ON purchase_return_items (return_id);
