---
name: user-data-portability
description: Keeping everything a user owns inside the JSON export, the JSON restore and cloud backups. Use whenever adding a table that accumulates user rows, adding a field to one, or touching GET/POST/DELETE /users/me/data (and its deprecated aliases /exports/json, /imports/json), the BackupPayload schema or internal/service/cloudbackup — and when the classification test fails on a new table.
---

# Everything a user owns leaves with them

A pilot's logbook is theirs. They must be able to export it, move to another NinerLog
installation — or a self-hosted one — restore it, and find everything still there. That is the
promise of `GET /users/me/data` and `POST /users/me/data`, and it holds only if every table that
accumulates user rows is in the payload. (`GET /exports/json` and `POST /imports/json` are
deprecated aliases served by the same code.)

It has been broken before. Contacts, custom currency rules, notification preferences and the
carried-forward hours baseline were all stored per user and none of them were in the export;
a pilot who moved installations silently lost their address book and every rule they had
written. Nothing failed — the export succeeded, it was just incomplete.

## One payload, three consumers

`cloudbackup.Payload` (`internal/service/cloudbackup/payload.go`) is the single definition of
what a backup contains. Three paths use it, and they must never diverge:

| Path | Uses |
| --- | --- |
| `GET /users/me/data` (`GET /exports/json`) | `APIHandler.BackupPayloadBuilder().Gather()`, written as indented JSON |
| Cloud backup run | the same builder's `BuildJSON()`, which gathers then gzips |
| `POST /users/me/data` (`POST /imports/json`) | decodes into `cloudbackup.Payload` and hands it to `cloudbackup.Restorer` |

`cmd/api/main.go` hands the cloud backup service the very builder the export handler uses, so
adding a section reaches both at once. The restorer is the half that does *not* update itself:
adding a field to `Payload` without restoring it produces a backup that looks complete and
silently drops data on restore.

The spec carries a mirror of the payload for generated clients: `components.schemas.BackupPayload`
and the `Backup*` schemas. `internal/service/cloudbackup/payload_spec_test.go` walks the Go types
against the YAML and fails on any JSON tag without a property, any property without a tag, and
any `required` list that disagrees with `omitempty`. Handlers keep using `cloudbackup.Payload`;
generated types are for clients.

## Adding a table

1. Add the section to `cloudbackup.Payload`, with a `json` tag in `camelCase`, and the matching
   property to `BackupPayload` in `api-spec/openapi.yaml` (reuse a shared component schema only
   if its JSON matches the model's tags and date formats; otherwise add a `Backup*` schema).
2. Gather it in `DefaultJSONBuilder.Gather` — sorted by a stable key, because the SHA-256 of
   the payload drives the "skip backup if unchanged" optimisation and unstable ordering makes
   every run look like a change. A nil service omits its section rather than failing the run.
3. Restore it: plan it in `restore_plan.go`, write it through a `repository.RestoreTx` method
   (SQL in `internal/repository/postgres/restore.go`, batched if it can reach thousands of
   rows), add it to `RestoreContent` and to the wipe if it is a collection, and add its counter
   to `RestoreResult` **and** to the `ImportJSONResult` schema in the spec.
4. Classify the table in `internal/service/cloudbackup/coverage_test.go`.
5. Extend `fullPayload()` in `restore_roundtrip_test.go`, and regenerate
   `testdata/backup-v<version>.json` if the section belongs in the committed backup.
6. Update the `/users/me/data` descriptions in `api-spec/openapi.yaml` and `docs/API.md`.

The classification test fails the build for any table in `db/migrations` that is in neither
list, so step 4 is not optional — it is how the rule stays true after everyone has forgotten
this file exists.

## What is legitimately exempt

Not every table is the user's to take. A table is exempt when it is:

- **a credential or session** — `refresh_tokens`, `password_reset_tokens`, WebAuthn
  credentials (bound to this origin and untransferable by design), OIDC identities and states;
- **bound to this installation** — backup destination credentials, flight signatures (they
  attest through this server's signing links and keys);
- **operator rather than user content** — the admin audit log, email delivery events and
  suppressions, announcements;
- **derived or transient** — deletion tombstones, idempotency keys, in-progress flight
  sessions, notification send history, past import runs.

Write the reason in the `exempt` map. "Not needed" is not a reason; the next person needs to
know whether it was a decision or an oversight.

Binary attachments (`document_files`) are exempt from the *JSON* payload because inlining
them would bloat it, not because they are unimportant — they have their own endpoints. If that
ever changes, it changes here too.

## Restore semantics

- **All or nothing.** The whole restore, and the wipe of `mode=replace`, run in one database
  transaction (`repository.RestoreStore.InRestoreTx`, serialised per user by an advisory lock).
  Any error rolls back and answers a generic message; details go to `slog`, never to the client.
- **Never a second copy.** `mode=restore` (the default) needs an empty account — no flights,
  aircraft, licences, credentials, contacts, custom currency rules or custom reports; the
  single-row settings do not count — and answers `409 account_not_empty` otherwise.
  `mode=replace` wipes those collections plus crew and import history first, and answers
  `409 replace_would_drop_attachments` while document files or signed flights exist, because
  those are exempt from the payload and a wipe would lose them.
- **Single-row settings replace.** Notification preferences and the flight baseline are one
  row per user, so a restore overwrites. Say so in the endpoint description.
- **IDs are always regenerated.** A backup must restore into any installation, including the
  one it came from.
- **Nothing installation-scoped rides along.** A custom currency rule's share token is unique
  across the installation that minted it, so the portable projection drops sharing state
  entirely and a restored rule is private until shared again. Apply the same reasoning to any
  new field that names a token, a URL, or another account. Flights drop `signatureId`.
- **Revalidate, never gate on limits.** A backup is user-supplied input, so it is checked on
  the way in — but a payload from any released export must restore. Per-account count limits
  (custom currency rules, custom reports) never apply. Something the database can store but
  today's validation rejects is restored and reported in `warnings`
  (`{section, index, message}`); a custom currency rule comes back with `enabled` and `notify`
  false. Only what the database cannot hold (over-long text, an unknown enum value) fails the
  restore, with a `400` naming the section and position.
- **Order matters.** Contacts are inserted before flights and crew so crew rows can link to
  them; a crew name matching no contact gets one (`contactsCreated`).
- **Bulk writes.** Flights, contacts and crew go in multi-row `INSERT`s in chunks, never one
  round trip per row. Everything else is bounded (a few thousand rows) and uses the ordinary
  repository `Create`, bound to the transaction.
- **Time and size.** 200 MiB body, 100,000 flights, a 5-minute request budget and a 4-minute
  `statement_timeout` inside the transaction; see `docs/API.md`.

## Version policy

`Payload.Version` is `<major>.<minor>` and the build writes `cloudbackup.PayloadVersion`
(`1.0`). A restore accepts a backup with no `version` and any major up to
`cloudbackup.SupportedMajorVersion`; a newer major answers `422 backup_version_unsupported`.
Unknown fields are ignored. Adding an optional section or field is not a new version. Changing
or removing the meaning of an existing one is: bump the major, keep reading the old shape, and
commit a fictional backup of every released version as `testdata/backup-v<version>.json`.
`TestRestore_CommittedBackupsStillRestore` restores every one of them and must stay green for
good — it is the proof that a pilot's old backup still works.
