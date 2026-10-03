-- Token revocation by counter rather than by clock.
--
-- Revocation compared a token's "issued at" (whole seconds, from the JWT) with a
-- tokens_valid_from timestamp. A token minted in the same second as a logout had to be
-- honoured — otherwise a fresh sign-in could reject itself — so a token stolen and used
-- within that second survived the logout. A counter has no such gap: logout and password
-- reset raise it, and a token carrying an older number is refused.
ALTER TABLE users
    ADD COLUMN IF NOT EXISTS session_version INTEGER NOT NULL DEFAULT 1;
