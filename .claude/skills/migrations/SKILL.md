---
name: migrations
description: Database schema changes in this repo — creating golang-migrate migration pairs, naming and numbering rules, the reversible-down requirement, the test-DB init script, and repository/model follow-up. Use whenever adding a table, column, index, or constraint.
---

# Database migrations

Schema lives in `db/migrations/`, applied by `golang-migrate` **automatically at startup**
(`m.Up()` in `cmd/api/main.go`, ignoring `ErrNoChange`). There is no sqlc; repositories in
`internal/repository/postgres` are hand-written parameterized SQL.

## Creating one

```bash
make migrate-create NAME=add_flight_custom_values   # scaffolds NNNNNN_name.{up,down}.sql
make migrate-check                                  # validate before committing
```

Rules enforced by `scripts/check-migrations.sh` (and by CI):

1. Filenames match `NNNNNN_name.(up|down).sql` — 6-digit version, lowercase snake_case name.
2. **No duplicate version numbers.** A duplicate makes `golang-migrate` refuse to open the
   source, which crashes the API at startup before it serves anything. This has happened before
   (see the fix for duplicate `000046`). When rebasing onto `main`, re-check your number.
3. Every version has both an `.up.sql` and a `.down.sql`. The down must actually reverse the up
   (`DROP TRIGGER IF EXISTS …` before `DROP TABLE IF EXISTS …`).

Never edit a migration that has already been merged — add a new one.

## Conventions

- `UUID PRIMARY KEY DEFAULT gen_random_uuid()`, `TIMESTAMPTZ NOT NULL DEFAULT NOW()` for
  `created_at`/`updated_at`, and `REFERENCES users(id) ON DELETE CASCADE` for user-owned rows.
- Attach the shared `update_updated_at_column()` trigger to tables with `updated_at`.
- Add the indexes the queries need (user-scoped lookups are almost always `(user_id, …)`).
- Durations are `INTEGER` **minutes**, never decimal hours (migration `000031` converted them).
- Explain non-obvious intent in a comment header at the top of the up-migration; the existing
  migrations do this and it is the main documentation of schema intent.

## Follow-up in the same change

- Update the domain struct in `internal/models/` and any text-length limits in
  `internal/models/validation.go`.
- Update the affected repository SQL in `internal/repository/postgres/` (column lists are
  explicit — a new column must be added to every relevant `SELECT`/`INSERT`/`UPDATE`).
- The integration test database is seeded by `scripts/seed-test-db.sh`, which applies the
  numbered migrations in order and records the resulting version in `schema_migrations`.
  There is no hand-maintained combined schema to keep in sync — a new migration is picked
  up automatically.
- Update `docs/DATA_MODEL.md` in the same PR.
