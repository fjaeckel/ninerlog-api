#!/usr/bin/env bash
#
# Seed a test database by applying db/migrations/*.up.sql in order.
#
# This replaces the hand-maintained combined schema that used to live at
# db/migrations/test_init.sql. That file had to be updated by hand for every
# migration and silently fell behind, so integration tests ran against a schema
# that no longer matched the repositories under test.
#
# After applying the migrations the script writes golang-migrate's
# schema_migrations bookkeeping, so an API process pointed at the same database
# sees it as fully migrated and starts cleanly instead of trying to re-apply
# migration 1.
#
# Usage:
#   scripts/seed-test-db.sh                 # via docker compose (default)
#   PSQL_CMD="psql -h 127.0.0.1 -p 5433 -U testuser -d ninerlog_test" \
#     scripts/seed-test-db.sh               # against any reachable Postgres
set -euo pipefail

cd "$(dirname "$0")/.."

MIGRATIONS_DIR="${MIGRATIONS_DIR:-db/migrations}"
PSQL_CMD="${PSQL_CMD:-docker compose -f docker-compose.test.yaml exec -T postgres-test psql -U testuser -d ninerlog_test}"

shopt -s nullglob
migrations=("$MIGRATIONS_DIR"/*.up.sql)
shopt -u nullglob

if [ ${#migrations[@]} -eq 0 ]; then
  echo "no migrations found in $MIGRATIONS_DIR" >&2
  exit 1
fi

# Sort by the numeric prefix so 000010 follows 000009 rather than 000001.
IFS=$'\n' migrations=($(printf '%s\n' "${migrations[@]}" | sort)); unset IFS

echo "Seeding test database from ${#migrations[@]} migrations..."
for f in "${migrations[@]}"; do
  if ! $PSQL_CMD -v ON_ERROR_STOP=1 -q < "$f"; then
    echo "FAILED applying $f" >&2
    exit 1
  fi
done

# Record the final version the way golang-migrate does, so `m.Up()` in the API
# is a no-op rather than an attempt to replay everything.
last="${migrations[-1]}"
version="$(basename "$last" | sed 's/^0*\([0-9]\+\)_.*/\1/')"
$PSQL_CMD -v ON_ERROR_STOP=1 -q <<SQL
CREATE TABLE IF NOT EXISTS schema_migrations (version BIGINT NOT NULL PRIMARY KEY, dirty BOOLEAN NOT NULL);
DELETE FROM schema_migrations;
INSERT INTO schema_migrations (version, dirty) VALUES ($version, false);
SQL

echo "Test database seeded (schema version $version)."
