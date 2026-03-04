-- UP
CREATE TABLE company_bank_accounts (
    id SERIAL PRIMARY KEY,

    company_id INT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,

    account_holder_name TEXT NOT NULL,
    bank_name TEXT NOT NULL,
    account_number TEXT NOT NULL,
    ifsc_code TEXT NOT NULL,
    branch TEXT,
    upi_id TEXT,

    is_default BOOLEAN DEFAULT FALSE,

    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX idx_company_bank_company ON company_bank_accounts(company_id);

