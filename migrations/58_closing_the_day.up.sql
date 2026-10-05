-- Closing the day: counting the stock, and counting the cash.
--
-- The app knew what the shop should have — what stock the books say is on the floor,
-- what cash the day's billing should have brought in. It had no way to record what was
-- actually there. So a shopkeeper who counted 48 boxes where the app said 52 had
-- nowhere to put that, and the app stayed confidently wrong.

-- MARK: - Stocktake

-- A count of the floor, as one session. Drafted while the counting happens — which in a
-- tile shop is an hour with a torch and a ladder — and applied once at the end.
CREATE TABLE IF NOT EXISTS stocktakes (
    id         BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    user_id    INTEGER NOT NULL,
    status     TEXT NOT NULL DEFAULT 'draft',
    note       TEXT,
    started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    applied_at TIMESTAMPTZ,
    CONSTRAINT stocktakes_status_check CHECK (status IN ('draft', 'applied', 'abandoned'))
);

CREATE INDEX IF NOT EXISTS stocktakes_company_idx ON stocktakes (company_id, started_at DESC);

-- One line per item counted.
--
-- `expected` is what the books said at the moment it was counted, kept rather than
-- looked up again later: it is what the person was comparing against with the item in
-- their hands, and it is what makes the variance mean something afterwards.
CREATE TABLE IF NOT EXISTS stocktake_lines (
    id           BIGSERIAL PRIMARY KEY,
    stocktake_id BIGINT NOT NULL REFERENCES stocktakes(id) ON DELETE CASCADE,
    item_id      BIGINT NOT NULL REFERENCES items(id) ON DELETE CASCADE,
    expected     INTEGER NOT NULL,
    counted      INTEGER NOT NULL CHECK (counted >= 0),
    counted_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT stocktake_lines_one_per_item UNIQUE (stocktake_id, item_id)
);

CREATE INDEX IF NOT EXISTS stocktake_lines_stocktake_idx ON stocktake_lines (stocktake_id);

-- MARK: - Cash closing

-- What the till should hold against what was counted out of it.
--
-- One per company per day: closing the same day twice is correcting the first count,
-- not a second event, so the row is replaced rather than added to.
CREATE TABLE IF NOT EXISTS day_closings (
    id            BIGSERIAL PRIMARY KEY,
    company_id    BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    user_id       INTEGER NOT NULL,
    closing_date  DATE NOT NULL DEFAULT CURRENT_DATE,
    -- What the day's cash payments add up to.
    expected_cash NUMERIC(12,2) NOT NULL DEFAULT 0,
    -- What was in the drawer.
    counted_cash  NUMERIC(12,2) NOT NULL DEFAULT 0,
    -- Counted less expected: negative is short, positive is over. Stored rather than
    -- derived so the figure cannot drift if a payment for that day is corrected later —
    -- the closing records what was found at the time.
    difference    NUMERIC(12,2) NOT NULL DEFAULT 0,
    note          TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT day_closings_one_per_day UNIQUE (company_id, closing_date)
);

CREATE INDEX IF NOT EXISTS day_closings_company_idx ON day_closings (company_id, closing_date DESC);

-- A stocktake writes a stock movement like anything else that moves stock, so the item's
-- history reads as one list however the change arrived.
ALTER TABLE stock_movements DROP CONSTRAINT IF EXISTS stock_movements_movement_type_check;
ALTER TABLE stock_movements ADD CONSTRAINT stock_movements_movement_type_check
    CHECK (movement_type IN (
        'restock', 'sale', 'adjustment', 'initial',
        'purchase', 'purchase_return', 'stocktake'
    ));
