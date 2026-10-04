-- Last authenticated request per account, for the admin user list.
ALTER TABLE users ADD COLUMN last_active_at TIMESTAMPTZ;

UPDATE users SET last_active_at = last_login_at WHERE last_login_at IS NOT NULL;
