-- The original UNIQUE constraint on clients.email was global (not scoped per user),
-- so two different users could never have a client with the same email, and any user
-- with more than one client lacking an email (including the Cash/UPI quick-sale
-- accounts, which are created with email = '') would hit a duplicate-key error on
-- the second such client.
ALTER TABLE clients DROP CONSTRAINT clients_email_key;

-- Replace it with a per-user, non-blank-only unique index: prevents a user from
-- having two clients with the same real email, while allowing any number of
-- clients (including quick-sale accounts) with a blank email, and allowing the
-- same email to be used by clients belonging to different users.
CREATE UNIQUE INDEX clients_user_id_email_unique
    ON clients (user_id, email)
    WHERE email <> '';
