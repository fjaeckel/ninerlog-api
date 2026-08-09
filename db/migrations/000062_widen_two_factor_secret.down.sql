-- Narrow users.two_factor_secret back to VARCHAR(64).
--
-- Every encrypted seed is 87 characters, so the ALTER would fail on any live
-- enrolment. Rolling back to a schema that cannot hold what the application
-- writes means 2FA cannot work anyway, so the enrolments that would block it
-- are cleared first and those users re-enrol — the same trade migration 61
-- makes, for the same reason.

UPDATE users
SET two_factor_enabled = FALSE,
    two_factor_secret  = NULL,
    recovery_codes     = NULL
WHERE two_factor_secret IS NOT NULL
  AND length(two_factor_secret) > 64;

ALTER TABLE users ALTER COLUMN two_factor_secret TYPE VARCHAR(64);

COMMENT ON COLUMN users.two_factor_secret IS 'Base32-encoded TOTP secret (encrypted at rest recommended)';
