DROP INDEX IF EXISTS idx_otp_email_active;
DROP INDEX IF EXISTS idx_reset_email_active;
ALTER TABLE otp_codes DROP COLUMN IF EXISTS attempts;
ALTER TABLE password_reset_tokens DROP COLUMN IF EXISTS attempts;
-- Truncating back to VARCHAR(6) would corrupt stored hashes; codes are short-lived so
-- the surviving rows are retired instead.
UPDATE otp_codes SET used = TRUE WHERE used = FALSE;
UPDATE password_reset_tokens SET used = TRUE WHERE used = FALSE;
ALTER TABLE otp_codes ALTER COLUMN code TYPE VARCHAR(6) USING LEFT(code, 6);
ALTER TABLE password_reset_tokens ALTER COLUMN code TYPE VARCHAR(6) USING LEFT(code, 6);
