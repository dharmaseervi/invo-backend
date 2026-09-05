-- Only is_verified is this migration's to undo — created_at already existed on
-- users from 01_users; that ADD COLUMN IF NOT EXISTS here was a no-op for it.
ALTER TABLE users DROP COLUMN IF EXISTS is_verified;
