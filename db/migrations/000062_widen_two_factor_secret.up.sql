-- Make users.two_factor_secret wide enough for the value that is actually
-- stored in it.
--
-- The column was sized for a plaintext base32 TOTP seed: 32 characters, so
-- VARCHAR(64) looked generous. An encrypted seed is not 32 characters. It is
-- the "enc:v1:" marker plus base64 of (12-byte nonce ‖ 32-byte ciphertext ‖
-- 16-byte GCM tag) — 87 characters, which Postgres rejects outright rather than
-- truncating.
--
-- The mismatch was invisible until now because encryption was optional and the
-- service-level tests use an in-memory repository with no column widths: only a
-- deployment that had actually set a TOTP key ever wrote an 87-character value,
-- and it got a database error at enrolment. With encryption mandatory, every
-- enrolment writes one, so this has to be fixed here rather than left to the
-- next person to trip over.
--
-- TEXT rather than a bigger VARCHAR: Postgres stores them identically, and a
-- number chosen to fit today's ciphertext is just the same bug waiting for the
-- next format change.

ALTER TABLE users ALTER COLUMN two_factor_secret TYPE TEXT;

COMMENT ON COLUMN users.two_factor_secret IS 'TOTP secret, AES-256-GCM encrypted at rest under a subkey of ENCRYPTION_KEY and marked with the enc:v1: prefix';
