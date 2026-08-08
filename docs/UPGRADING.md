# Upgrading

Breaking changes, newest first, with what an operator has to do about them.

## One `ENCRYPTION_KEY` for everything encrypted at rest

**Breaking.** `TOTP_ENCRYPTION_KEY` and `BACKUP_CREDENTIALS_KEY` are removed.
Every key the server uses at rest is now derived from a single `ENCRYPTION_KEY`,
and that key is **required** — the API will not start without it.

Before, each use brought its own optional key, and a missing one meant "store it
in the clear and log a warning". That produced three secrets to manage, two
plaintext fallbacks nobody noticed they were relying on, and no key at all for
licence and credential files. Now there is one secret, mandatory, with each use
deriving its own subkey via HKDF-SHA256, so no two uses share key bytes and none
of them is the master.

### What to do

1. **Generate a key** and put it in the environment:

   ```bash
   openssl rand -base64 32
   ```

   ```
   ENCRYPTION_KEY=<the value>
   ```

   Keep a copy wherever the database password lives. Data sealed with it cannot
   be recovered without it — not from a database dump, not by us, and there is no
   reset.

2. **Unset `TOTP_ENCRYPTION_KEY` and `BACKUP_CREDENTIALS_KEY`.** The server
   refuses to start while either is still set, rather than ignoring it and
   letting you discover the consequence from a locked-out pilot. The data they
   protected is cleared automatically — see below, and tell your users first.

3. **If you use cloud backups**, set `CLOUD_BACKUPS_ENABLED=true`. The subsystem
   used to switch itself on when its key was present; with one shared key that
   would mean setting `ENCRYPTION_KEY` silently started a scheduler and a set of
   outbound-connecting providers, so it has an explicit switch now. It is off by
   default, which is what "no backup key configured" meant before.

### What breaks, and what does not

Migration 61 does the cleanup for you, in one transaction, before the API serves
its first request. You do not have to run any SQL by hand — but you do have to
tell your users, because they will notice.

**Every 2FA enrolment is cleared.** `two_factor_enabled` goes false and the
secret and recovery codes are dropped, for all accounts. Users sign in with their
password and re-enrol from scratch.

This is deliberately unconditional. Secrets that were encrypted under
`TOTP_ENCRYPTION_KEY` cannot be read under the new scheme, and an account whose
enrolment exists but cannot be verified is an account that cannot log in at all,
with no self-service route back — so leaving those rows in place is worse than
clearing them. Secrets from an installation that never set that key were stored
in the clear and could technically have survived, but "your seed lived because it
was the one that was not encrypted" is not a rule worth keeping, and the read
path no longer accepts an unencrypted secret at all.

**Every session is ended.** All refresh tokens are deleted. A session minted
after a second factor represents an authentication that has just been
invalidated, so it must not outlive it — and neither should a stolen refresh
token. Everyone signs in again.

**Every backup destination is deleted**, along with its run history. The stored
credentials were sealed with `BACKUP_CREDENTIALS_KEY` and are now opaque bytes;
the row cannot be repaired, only re-created with the credentials entered again.

**Licence and credential files** stored before this release are deleted by
migration 60. Nothing inside the database can encrypt them — the key lives in the
application's environment — and the alternative was a permanent second storage
format for rows still in the clear. The feature is days old and no released
version ships it, so this is test data; re-upload after deploying.

**Passkeys are untouched.** WebAuthn credentials were never encrypted with any of
the removed keys, so there is nothing wrong with them and no reason to make
anyone re-register a security key. Users who sign in with a passkey are not
affected beyond having to do it again once, since their session was ended.

**Passwords, flights, licences, aircraft and every other record** are likewise
unaffected.

### Rolling back

Migration 61 has no meaningful down step: the cleared values are gone from the
database and the keys that could have read them are gone from the environment, so
there is nothing to restore. It rolls back as a no-op rather than failing, so a
`migrate down` across it is not blocked.

Migration 60's down step deletes stored licence/credential files, for the reason
above: after a rollback nothing can read AES-GCM ciphertext out of `data`, and
serving those bytes to a browser as if they were a JPEG would be worse than
losing them. Download anything you want to keep first, or restore a dump taken
before the upgrade.
