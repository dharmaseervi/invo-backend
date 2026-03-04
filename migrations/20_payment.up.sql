CREATE TABLE payments (
	id BIGSERIAL PRIMARY KEY,
	company_id BIGINT NOT NULL,
	client_id BIGINT NOT NULL,
	invoice_id BIGINT NOT NULL,
	amount NUMERIC(12,2) NOT NULL,
	payment_method VARCHAR(50),
	reference TEXT,
	notes TEXT,
	created_at TIMESTAMP DEFAULT NOW()
);
CREATE INDEX idx_payments_company ON payments(company_id);
CREATE INDEX idx_payments_client ON payments(client_id);
CREATE INDEX idx_payments_invoice ON payments(invoice_id);