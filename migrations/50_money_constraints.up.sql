-- Application validation is the first line of defence, but these amounts must never be
-- negative regardless of which code path writes them. A negative invoice total or a
-- negative payment is not a recoverable state for accounting - it has to be impossible
-- at the storage layer too.
--
-- NOT VALID: the constraint is enforced for every new and updated row immediately,
-- without scanning existing data. Any historical row that violates it stays put rather
-- than blocking the deploy; validate separately once the data is known to be clean.

ALTER TABLE invoices
    ADD CONSTRAINT invoices_amounts_non_negative CHECK (
        subtotal >= 0 AND tax >= 0 AND total >= 0
        AND COALESCE(discount, 0) >= 0
        AND COALESCE(paid_amount, 0) >= 0
    ) NOT VALID;

ALTER TABLE invoice_items
    ADD CONSTRAINT invoice_items_amounts_non_negative CHECK (
        qty > 0 AND rate >= 0 AND total >= 0 AND COALESCE(discount, 0) >= 0
    ) NOT VALID;

ALTER TABLE estimates
    ADD CONSTRAINT estimates_amounts_non_negative CHECK (
        subtotal >= 0 AND tax >= 0 AND total >= 0 AND COALESCE(discount, 0) >= 0
    ) NOT VALID;

ALTER TABLE estimate_items
    ADD CONSTRAINT estimate_items_amounts_non_negative CHECK (
        qty > 0 AND rate >= 0 AND total >= 0 AND COALESCE(discount, 0) >= 0
    ) NOT VALID;

-- A payment or an allocation of zero or less is meaningless and would corrupt a
-- client's outstanding balance.
ALTER TABLE payments
    ADD CONSTRAINT payments_amount_positive CHECK (amount > 0) NOT VALID;

ALTER TABLE payment_allocations
    ADD CONSTRAINT payment_allocations_amount_positive CHECK (amount > 0) NOT VALID;

-- A ledger row records money moving one way or the other, never both and never
-- backwards; the running balance itself may legitimately be negative.
ALTER TABLE ledger_entries
    ADD CONSTRAINT ledger_entries_amounts_non_negative CHECK (
        COALESCE(debit, 0) >= 0 AND COALESCE(credit, 0) >= 0
    ) NOT VALID;

ALTER TABLE items
    ADD CONSTRAINT items_prices_non_negative CHECK (
        price >= 0 AND COALESCE(cost_price, 0) >= 0
    ) NOT VALID;
