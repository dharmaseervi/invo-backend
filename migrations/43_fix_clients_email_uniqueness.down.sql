DROP INDEX IF EXISTS clients_user_id_email_unique;

-- Restoring the original global UNIQUE constraint will fail if any duplicate or
-- blank emails were created while the fixed constraint was in effect — that data
-- would need to be cleaned up manually before this constraint can be re-added.
ALTER TABLE clients ADD CONSTRAINT clients_email_key UNIQUE (email);
