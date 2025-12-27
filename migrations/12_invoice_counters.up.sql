CREATE TABLE invoice_counters (
    id SERIAL PRIMARY KEY,
    company_id INT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    financial_year VARCHAR(9) NOT NULL, -- FY24-25
    next_number INT NOT NULL DEFAULT 1,
    UNIQUE (company_id, financial_year)
);
