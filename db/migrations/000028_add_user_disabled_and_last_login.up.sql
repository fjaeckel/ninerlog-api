-- Add disabled flag and last_login_at to users table for admin user management
ALTER TABLE users ADD COLUMN disabled BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE users ADD COLUMN last_login_at TIMESTAMPTZ;
