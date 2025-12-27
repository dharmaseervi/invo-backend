-- =========================
-- Add workflow + payment fields to invoices
-- =========================

ALTER TABLE invoices
ADD COLUMN status VARCHAR(20) NOT NULL DEFAULT 'draft',
ADD COLUMN paid_amount NUMERIC(10,2) NOT NULL DEFAULT 0,
ADD COLUMN remaining_amount NUMERIC(10,2),
ADD COLUMN notes TEXT;
