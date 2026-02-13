-- Add 2FA fields to users table
ALTER TABLE users
    ADD COLUMN two_factor_enabled BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN two_factor_secret VARCHAR(64),
    ADD COLUMN recovery_codes TEXT[];

COMMENT ON COLUMN users.two_factor_enabled IS 'Whether TOTP 2FA is active';
COMMENT ON COLUMN users.two_factor_secret IS 'Base32-encoded TOTP secret (encrypted at rest recommended)';
COMMENT ON COLUMN users.recovery_codes IS 'Hashed one-time recovery codes';
