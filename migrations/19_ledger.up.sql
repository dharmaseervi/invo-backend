CREATE TABLE ledger_entries (
    id BIGSERIAL PRIMARY KEY,

    company_id BIGINT NOT NULL,
    client_id BIGINT NOT NULL,

    source_type TEXT NOT NULL CHECK (
        source_type IN ('INVOICE', 'PAYMENT', 'CREDIT_NOTE', 'ADJUSTMENT')
    ),

    source_id BIGINT NOT NULL,

    debit NUMERIC(12,2) DEFAULT 0,
    credit NUMERIC(12,2) DEFAULT 0,

    balance NUMERIC(12,2) NOT NULL,

    description TEXT,

    created_at TIMESTAMP NOT NULL DEFAULT NOW(),

    CONSTRAINT fk_ledger_company FOREIGN KEY (company_id) REFERENCES companies(id),
    CONSTRAINT fk_ledger_client FOREIGN KEY (client_id) REFERENCES clients(id)
);

CREATE INDEX idx_ledger_client ON ledger_entries(client_id);
CREATE INDEX idx_ledger_company ON ledger_entries(company_id);