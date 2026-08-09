-- Clear every secret that the removed per-purpose keys used to protect.
--
-- Migration 60 moved all at-rest encryption onto subkeys of ENCRYPTION_KEY.
-- Anything sealed under the keys it replaced -- TOTP_ENCRYPTION_KEY,
-- BACKUP_CREDENTIALS_KEY -- can no longer be decrypted, and leaving those rows
-- in place is worse than clearing them: a pilot with a dead 2FA enrolment
-- cannot log in at all and has no self-service route back, and a backup
-- destination with unreadable credentials fails silently on its schedule.
--
-- Doing it here rather than at first boot, or in a hand-run SQL snippet in the
-- upgrade notes:
--
--   * it runs exactly once, in a transaction, before the API serves its first
--     request -- so no pilot can hit the window where their enrolment exists
--     but cannot be verified;
--   * it runs on every deployment, including the ones whose operator never
--     reads the upgrade notes, which are the deployments that need it most;
--   * an operator who skips a manual step gets locked-out users, and the
--     symptom (login fails at the second factor) does not point at its cause.
--
-- DESTRUCTIVE. Everything below is unrecoverable by design -- that is the point,
-- since none of it can be read any more.

-- 2FA enrolments. Every account is dropped back to password-only and must
-- re-enrol; the clients handle a disabled enrolment as the ordinary state it is.
--
-- Deliberately not conditional on whether a given secret was encrypted. A
-- deployment that never set TOTP_ENCRYPTION_KEY stored its secrets in the clear
-- and those would technically still verify, but "your seed survived because it
-- was the one stored unencrypted" is not a rule worth keeping in the schema, and
-- the read path no longer accepts an unprefixed secret at all.
--
-- Recovery codes go with the enrolment they belong to. They are the way back in
-- when an authenticator is lost, so leaving them behind a disabled enrolment
-- would leave single-use credentials sitting in the table with nothing to
-- unlock.
UPDATE users
SET two_factor_enabled = FALSE,
    two_factor_secret  = NULL,
    recovery_codes     = NULL
WHERE two_factor_enabled
   OR two_factor_secret IS NOT NULL
   OR recovery_codes IS NOT NULL;

-- Sessions minted while those enrolments were live. A refresh token issued
-- after a second factor represents an authentication we have just invalidated,
-- so it must not outlive it: without this, every already-signed-in pilot keeps a
-- session whose 2FA step can no longer be repeated, and an attacker holding a
-- stolen refresh token keeps one too. Everyone signs in again with their
-- password.
DELETE FROM refresh_tokens;

-- Backup destinations. credentials_enc / credentials_nonce were sealed with
-- BACKUP_CREDENTIALS_KEY and are now opaque bytes; the row cannot be repaired,
-- only re-created with the credentials typed in again. Runs cascade with the
-- destination, taking the history of backups that can no longer be reproduced.
DELETE FROM backup_destinations;
