-- Payment corrections, advances and refunds.
--
-- Three things a shop does every week that the app had no answer for: a payment
-- recorded against the wrong customer or invoice, a deposit taken before any invoice
-- exists, and money handed back.
--
-- A wrong payment is never deleted. The row stays, marked reversed, with a reversing
-- entry in the ledger — a books' history is the thing a customer argues from, and a row
-- that vanishes cannot be explained to them.
ALTER TABLE payments
    ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'recorded',
    ADD COLUMN IF NOT EXISTS reversed_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS reversal_reason TEXT,
    -- What the payment did not settle: the advance a customer is holding with this
    -- shop. Derived from the allocations, kept here so a list of payments does not need
    -- a join to show it.
    ADD COLUMN IF NOT EXISTS unapplied_amount NUMERIC(12,2) NOT NULL DEFAULT 0;

ALTER TABLE payments
    DROP CONSTRAINT IF EXISTS payments_status_check;
ALTER TABLE payments
    ADD CONSTRAINT payments_status_check CHECK (status IN ('recorded', 'reversed'));

-- Money going back to a customer: a refund against a credit note, or returning an
-- advance they are not going to use. Separate from payments because it is the opposite
-- direction, and summing a column that can be either way round is how a cash figure
-- ends up wrong.
CREATE TABLE IF NOT EXISTS refunds (
    id              BIGSERIAL PRIMARY KEY,
    company_id      BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    client_id       BIGINT NOT NULL REFERENCES clients(id) ON DELETE CASCADE,
    -- The credit note this returns money against, when there is one. A refund of an
    -- unused advance has none.
    credit_note_id  BIGINT REFERENCES credit_notes(id) ON DELETE SET NULL,
    amount          NUMERIC(12,2) NOT NULL CHECK (amount > 0),
    method          VARCHAR(50) NOT NULL,
    reference       TEXT,
    notes           TEXT,
    refund_date     DATE NOT NULL DEFAULT CURRENT_DATE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS refunds_company_date_idx ON refunds (company_id, refund_date DESC, id DESC);
CREATE INDEX IF NOT EXISTS refunds_client_idx ON refunds (client_id);

-- The ledger's source types are constrained, and the two new kinds of entry belong in
-- that list: a reversal undoing a payment, and a refund paid out.
ALTER TABLE ledger_entries
    DROP CONSTRAINT IF EXISTS ledger_entries_source_type_check;
ALTER TABLE ledger_entries
    ADD CONSTRAINT ledger_entries_source_type_check
    CHECK (source_type IN ('INVOICE', 'PAYMENT', 'CREDIT_NOTE', 'ADJUSTMENT', 'PAYMENT_REVERSAL', 'REFUND'));
