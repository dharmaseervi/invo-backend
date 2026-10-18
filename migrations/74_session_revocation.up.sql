-- Keep shipped access tokens working until expiry, unless the user explicitly
-- revokes devices. New access tokens are bound to an independently revocable sid.
ALTER TABLE users ADD COLUMN legacy_tokens_revoked BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE sessions ADD COLUMN session_version INTEGER;
-- Existing refresh sessions created before a reset/logout must not be upgraded
-- into the current authentication version. Those issued before the cutoff die.
DELETE FROM sessions s USING users u WHERE s.user_id=u.id AND s.created_at<u.tokens_valid_from;
UPDATE sessions s SET session_version=u.session_version FROM users u WHERE s.user_id=u.id;
ALTER TABLE sessions ALTER COLUMN session_version SET NOT NULL;
