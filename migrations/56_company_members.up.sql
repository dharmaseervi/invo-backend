-- Staff: letting somebody else work in the shop's books without handing over the owner's
-- login.
--
-- Until now an account and a business were the same thing. A shop with a counter boy and
-- a manager had one password between them, so everybody could see the day's takings, edit
-- a price, or delete an invoice — and nothing recorded who did.

CREATE TABLE IF NOT EXISTS company_members (
    id         BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,

    -- owner   — the business is theirs: everything, including staff and settings.
    -- manager — runs the shop day to day, but cannot change who works here.
    -- staff   — bills customers and takes payments. No costs, no reports, no deleting.
    role       TEXT NOT NULL CHECK (role IN ('owner', 'manager', 'staff')),

    -- What the owner calls them. The login is an email, which in a shop is often one
    -- the owner made up for them, and "ramesh@..." is not a name on a screen.
    name       VARCHAR(255) NOT NULL DEFAULT '',
    added_by   INTEGER REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT company_members_one_per_company UNIQUE (company_id, user_id)
);

CREATE INDEX IF NOT EXISTS company_members_user_idx ON company_members (user_id);
CREATE INDEX IF NOT EXISTS company_members_company_idx ON company_members (company_id, role);

-- Every existing business gets its owner as a member, so the row that grants access is
-- the same row for everybody from here on. companies.user_id stays as it was: it is
-- what says whose business it is, and the account deletion path still reads it.
INSERT INTO company_members (company_id, user_id, role, name)
SELECT c.id, c.user_id, 'owner', ''
FROM companies c
WHERE c.user_id IS NOT NULL
ON CONFLICT (company_id, user_id) DO NOTHING;

-- One owner per company, enforced rather than assumed: a second owner row would mean two
-- people who can remove each other.
CREATE UNIQUE INDEX IF NOT EXISTS company_members_single_owner
    ON company_members (company_id) WHERE role = 'owner';
