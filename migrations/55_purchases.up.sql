-- Purchases: who the shop buys from, what it bought, and what it still owes them.
--
-- The app could only ever see one side of a business. Stock arrived through a restock
-- with no record of where it came from or what it cost, so there was nothing to check a
-- supplier's statement against and no way to know what the shop owed.

CREATE TABLE IF NOT EXISTS suppliers (
    id          BIGSERIAL PRIMARY KEY,
    company_id  BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    user_id     BIGINT NOT NULL,
    name        VARCHAR(255) NOT NULL,
    phone       VARCHAR(20),
    email       VARCHAR(255),
    gstin       VARCHAR(15),
    address     TEXT,
    city        VARCHAR(100),
    state       VARCHAR(100),
    pincode     VARCHAR(10),
    notes       TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS suppliers_company_name_idx ON suppliers (company_id, name, id);

-- A bill from a supplier. bill_number is theirs, not ours: two suppliers can easily use
-- the same number, so it is unique per supplier rather than per company.
CREATE TABLE IF NOT EXISTS purchase_bills (
    id               BIGSERIAL PRIMARY KEY,
    company_id       BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    user_id          BIGINT NOT NULL,
    supplier_id      BIGINT NOT NULL REFERENCES suppliers(id) ON DELETE RESTRICT,
    bill_number      VARCHAR(100) NOT NULL,
    bill_date        DATE NOT NULL DEFAULT CURRENT_DATE,
    due_date         DATE,
    subtotal         NUMERIC(12,2) NOT NULL DEFAULT 0,
    tax              NUMERIC(12,2) NOT NULL DEFAULT 0,
    total            NUMERIC(12,2) NOT NULL DEFAULT 0,
    paid_amount      NUMERIC(12,2) NOT NULL DEFAULT 0,
    remaining_amount NUMERIC(12,2) NOT NULL DEFAULT 0,
    status           TEXT NOT NULL DEFAULT 'unpaid',
    notes            TEXT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT purchase_bills_status_check CHECK (status IN ('unpaid', 'partial', 'paid', 'cancelled')),
    CONSTRAINT purchase_bills_number_per_supplier UNIQUE (supplier_id, bill_number)
);

CREATE INDEX IF NOT EXISTS purchase_bills_company_date_idx
    ON purchase_bills (company_id, bill_date DESC, id DESC);

CREATE TABLE IF NOT EXISTS purchase_bill_items (
    id        BIGSERIAL PRIMARY KEY,
    bill_id   BIGINT NOT NULL REFERENCES purchase_bills(id) ON DELETE CASCADE,
    item_id   BIGINT NOT NULL REFERENCES items(id) ON DELETE RESTRICT,
    qty       INTEGER NOT NULL CHECK (qty > 0),
    rate      NUMERIC(12,2) NOT NULL CHECK (rate >= 0),
    tax_rate  NUMERIC(5,2) NOT NULL DEFAULT 0,
    total     NUMERIC(12,2) NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS purchase_bill_items_bill_idx ON purchase_bill_items (bill_id);

-- Money paid to a supplier, against a bill or on account.
CREATE TABLE IF NOT EXISTS supplier_payments (
    id          BIGSERIAL PRIMARY KEY,
    company_id  BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    supplier_id BIGINT NOT NULL REFERENCES suppliers(id) ON DELETE CASCADE,
    bill_id     BIGINT REFERENCES purchase_bills(id) ON DELETE SET NULL,
    amount      NUMERIC(12,2) NOT NULL CHECK (amount > 0),
    method      VARCHAR(50) NOT NULL,
    reference   TEXT,
    notes       TEXT,
    paid_on     DATE NOT NULL DEFAULT CURRENT_DATE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS supplier_payments_company_idx
    ON supplier_payments (company_id, paid_on DESC, id DESC);
CREATE INDEX IF NOT EXISTS supplier_payments_supplier_idx ON supplier_payments (supplier_id);

-- Stock movements record why a quantity changed, and a purchase is a new reason —
-- distinct from a restock, which is somebody typing a number in by hand.
ALTER TABLE stock_movements
    DROP CONSTRAINT IF EXISTS stock_movements_movement_type_check;
ALTER TABLE stock_movements
    ADD CONSTRAINT stock_movements_movement_type_check
    CHECK (movement_type IN ('restock', 'sale', 'adjustment', 'initial', 'purchase', 'purchase_return'));
