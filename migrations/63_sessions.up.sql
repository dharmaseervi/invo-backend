-- Device sessions — tracks every active login so users can see and revoke them.
--
-- A session is created at login and holds a hashed 90-day refresh token.
-- The access token (JWT) stays short-lived at 7 days; the refresh token is the
-- long-lived credential that issues new access tokens without re-entering a password.

CREATE TABLE IF NOT EXISTS sessions (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id            INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    refresh_token_hash TEXT    NOT NULL UNIQUE,
    device_name        TEXT    NOT NULL DEFAULT '',
    platform           TEXT    NOT NULL DEFAULT 'ios',
    ip_address         TEXT    NOT NULL DEFAULT '',
    last_seen          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at         TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS sessions_user_idx        ON sessions (user_id);
CREATE INDEX IF NOT EXISTS sessions_expires_at_idx  ON sessions (expires_at);
