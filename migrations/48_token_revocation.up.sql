-- JWTs are stateless, so logging out or resetting a password left every previously
-- issued token usable until it expired on its own. This records the moment a user's
-- existing tokens stop being accepted; anything issued before it is rejected.
--
-- The default is the epoch, not NOW(): defaulting to the migration time would make
-- every token already in circulation invalid the instant this deploys, signing out
-- every user mid-session. Epoch means "nothing revoked yet", which is the truth.
ALTER TABLE users
    ADD COLUMN IF NOT EXISTS tokens_valid_from TIMESTAMPTZ NOT NULL
    DEFAULT TIMESTAMPTZ '1970-01-01 00:00:00+00';
