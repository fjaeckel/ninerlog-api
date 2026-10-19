-- users.is_admin: per-account admin flag; any number of accounts may hold it.
-- Accounts matching ADMIN_EMAIL are promoted by the API at startup.
ALTER TABLE users ADD COLUMN is_admin BOOLEAN NOT NULL DEFAULT false;

CREATE INDEX idx_users_is_admin ON users(id) WHERE is_admin;
