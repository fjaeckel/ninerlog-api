---
name: user-data-portability
description: Keeping everything a user owns inside the JSON export, the JSON import and cloud backups. Use whenever adding a table that accumulates user rows, adding a field to one, or touching GET /exports/json, POST /imports/json or internal/service/cloudbackup — and when the classification test fails on a new table.
---

# Everything a user owns leaves with them

A pilot's logbook is theirs. They must be able to export it, move to another NinerLog
installation — or a self-hosted one — restore it, and find everything still there. That is the
promise of `GET /exports/json` and `POST /imports/json`, and it holds only if every table that
accumulates user rows is in the payload.

It has been broken before. Contacts, custom currency rules, notification preferences and the
carried-forward hours baseline were all stored per user and none of them were in the export;
a pilot who moved installations silently lost their address book and every rule they had
written. Nothing failed — the export succeeded, it was just incomplete.

## One payload, three consumers

`cloudbackup.Payload` (`internal/service/cloudbackup/payload.go`) is the single definition of
what a backup contains. Three paths use it, and they must never diverge:

| Path | Uses |
| --- | --- |
| `GET /exports/json` | `APIHandler.BackupPayloadBuilder().Gather()`, written as indented JSON |
| Cloud backup run | the same builder's `BuildJSON()`, which gathers then gzips |
| `POST /imports/json` | decodes into `importJSONBackup` and restores each section |

`cmd/api/main.go` hands the cloud backup service the very builder the export handler uses, so
adding a section reaches both at once. The importer is the half that does *not* update itself:
adding a field to `Payload` without restoring it produces a backup that looks complete and
silently drops data on restore.

## Adding a table

1. Add the section to `cloudbackup.Payload`, with a `json` tag in `camelCase`.
2. Gather it in `DefaultJSONBuilder.Gather` — sorted by a stable key, because the SHA-256 of
   the payload drives the "skip backup if unchanged" optimisation and unstable ordering makes
   every run look like a change. A nil service omits its section rather than failing the run.
3. Restore it in `ImportDataJSON`, and add its counter to `importJSONSummary` **and** to the
   `ImportJSONResult` schema in the spec.
4. Classify the table in `internal/service/cloudbackup/coverage_test.go`.
5. Update the `/exports/json` and `/imports/json` descriptions in `api-spec/openapi.yaml`.

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

- **Additive by default.** A restore never deletes existing data. Collections append;
  duplicates are skipped, not merged (aircraft by registration, contacts by name).
- **Single-row settings replace.** Notification preferences and the flight baseline are one
  row per user, so a restore overwrites. Say so in the endpoint description.
- **IDs are always regenerated.** A backup must restore into any installation, including the
  one it came from.
- **Nothing installation-scoped rides along.** A custom currency rule's share token is unique
  across the installation that minted it, so the portable projection drops sharing state
  entirely and a restored rule is private until shared again. Apply the same reasoning to any
  new field that names a token, a URL, or another account.
- **Restore through the service, not the repository.** Rules are revalidated and quotas
  applied on the way in; a backup is user-supplied input like any other request body.
- **Order matters.** Contacts are restored before flights so the crew linker matches a crew
  name against the contact the backup carried instead of creating a bare one.
