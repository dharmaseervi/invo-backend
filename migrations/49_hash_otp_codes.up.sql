-- One-time codes were stored in plaintext with no limit on guesses. Anyone with read
-- access to the database could sign in as any user, and a six-digit code with unlimited
-- attempts is guessable in minutes over the network.
--
-- Codes are now stored as bcrypt hashes, so the column has to hold 60 characters rather
-- than 6, and each row counts failed attempts so it can be burned after a few.

ALTER TABLE otp_codes ALTER COLUMN code TYPE TEXT;
ALTER TABLE otp_codes ADD COLUMN IF NOT EXISTS attempts INT NOT NULL DEFAULT 0;

ALTER TABLE password_reset_tokens ALTER COLUMN code TYPE TEXT;
ALTER TABLE password_reset_tokens ADD COLUMN IF NOT EXISTS attempts INT NOT NULL DEFAULT 0;

-- Codes already issued are plaintext and can never match a hash comparison. They expire
-- within ten minutes anyway, so retiring them now avoids leaving rows that silently fail
-- to verify; affected users simply request a new code.
UPDATE otp_codes SET used = TRUE WHERE used = FALSE;
UPDATE password_reset_tokens SET used = TRUE WHERE used = FALSE;

-- Verification looks up the newest unused code for an address.
CREATE INDEX IF NOT EXISTS idx_otp_email_active ON otp_codes(email, created_at DESC) WHERE used = FALSE;
CREATE INDEX IF NOT EXISTS idx_reset_email_active ON password_reset_tokens(email, created_at DESC) WHERE used = FALSE;
