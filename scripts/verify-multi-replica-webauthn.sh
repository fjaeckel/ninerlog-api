#!/usr/bin/env bash
#
# Multi-replica WebAuthn acceptance test.
#
# Runs two API instances against one database and drives passkey ceremonies with
# `begin` and `finish` deliberately landing on different instances. This is the
# scenario the Postgres-backed ceremony store exists for: with process-local
# state every one of these assertions fails.
#
# What it proves:
#   1. A session opened on instance A is consumable on instance B (both flows).
#   2. Consumption is single-use *across* instances — a handle burned on B is
#      dead on A.
#   3. A ceremony survives the death of the process that started it.
#   4. An expired handle is refused while its row is still physically present,
#      i.e. by the expires_at predicate rather than by the cleanup job.
#   5. A registration handle cannot be redeemed by a different account.
#   6. No raw handle reaches any instance's log output.
#
# What it does NOT prove: attestation/assertion verification, which needs a real
# authenticator. Ceremonies here are finished with a deliberately invalid
# credential payload, so the *distinguishing* signal is which error comes back:
#
#   "Invalid ... response"    -> the session was found and consumed (shared state)
#   "... session expired ..." -> the session was not usable (rejected)
#
# Usage:
#   scripts/verify-multi-replica-webauthn.sh
#
# Env (all optional):
#   PGHOST/PGPORT/PGUSER/PGDATABASE  Postgres connection (default 127.0.0.1:5433)
#   PORT_A / PORT_B                  instance ports (default 3001 / 3002)
set -uo pipefail

PGHOST="${PGHOST:-127.0.0.1}"
PGPORT="${PGPORT:-5433}"
PGUSER="${PGUSER:-testuser}"
PGDATABASE="${PGDATABASE:-ninerlog_multireplica}"
PORT_A="${PORT_A:-3001}"
PORT_B="${PORT_B:-3002}"
PORT_C="${PORT_C:-3003}"

BIN=./bin/ninerlog-api
WORKDIR="$(mktemp -d)"
PIDS=()
pass=0
fail=0

cleanup() {
  for pid in "${PIDS[@]:-}"; do kill "$pid" 2>/dev/null || true; done
  wait 2>/dev/null || true
  rm -rf "$WORKDIR"
}
trap cleanup EXIT

ok()   { printf '  \033[32mPASS\033[0m %s\n' "$1"; pass=$((pass+1)); }
bad()  { printf '  \033[31mFAIL\033[0m %s\n'   "$1"; fail=$((fail+1)); }
note() { printf '\n\033[1m%s\033[0m\n' "$1"; }

# assert_contains <needle> <haystack> <description>
assert_contains() {
  case "$2" in *"$1"*) ok "$3" ;; *) bad "$3"; printf '        got: %s\n' "$2" ;; esac
}

[ -x "$BIN" ] || { echo "build first: make build"; exit 1; }

note "Preparing database $PGDATABASE"
psql -h "$PGHOST" -p "$PGPORT" -U "$PGUSER" -d postgres -q \
  -c "DROP DATABASE IF EXISTS $PGDATABASE;" -c "CREATE DATABASE $PGDATABASE;" || exit 1

export DATABASE_URL="postgresql://$PGUSER@$PGHOST:$PGPORT/$PGDATABASE?sslmode=disable"
export JWT_SECRET="multi-replica-verification-jwt-secret-not-for-production-01"
export REFRESH_SECRET="multi-replica-verification-refresh-secret-not-for-prod-02"
export WEBAUTHN_RP_ID="localhost"
export WEBAUTHN_RP_ORIGINS="http://localhost:5173"
export MIGRATIONS_PATH="${MIGRATIONS_PATH:-db/migrations}"
export DISABLE_RATE_LIMIT=true
export AIRPORT_REFRESH_INTERVAL=off

start_instance() { # <port> <logfile>
  env PORT="$1" "$BIN" > "$2" 2>&1 &
  PIDS+=($!)
  for _ in $(seq 1 40); do
    [ "$(curl -s -o /dev/null -w '%{http_code}' "http://127.0.0.1:$1/health")" = "200" ] && return 0
    sleep 1
  done
  echo "instance on port $1 failed to become healthy; log:"; tail -20 "$2"; return 1
}

note "Starting two instances against one database"
start_instance "$PORT_A" "$WORKDIR/a.log" || exit 1   # applies migrations
start_instance "$PORT_B" "$WORKDIR/b.log" || exit 1
A="http://127.0.0.1:$PORT_A/api/v1"
B="http://127.0.0.1:$PORT_B/api/v1"
echo "  A=$A  B=$B"

jsonfield() { python3 -c "import json,sys;print(json.load(open(sys.argv[1])).get(sys.argv[2],''))" "$1" "$2"; }

mkuser() { # <email> -> prints access token
  curl -s -X POST "$A/auth/register" -H 'Content-Type: application/json' \
    -d "{\"email\":\"$1\",\"password\":\"SecurePass123!\",\"name\":\"MR User\"}" -o /dev/null
  curl -s -X POST "$A/auth/login" -H 'Content-Type: application/json' \
    -d "{\"email\":\"$1\",\"password\":\"SecurePass123!\"}" -o "$WORKDIR/login.json"
  jsonfield "$WORKDIR/login.json" accessToken
}

STAMP=$(date +%s)
TOKEN=$(mkuser "mr-$STAMP@example.com")
[ -n "$TOKEN" ] || { echo "could not obtain a token"; exit 1; }

begin_reg() { # <base> <token> -> prints sessionId
  curl -s -X POST "$1/auth/webauthn/register/options" -H "Authorization: Bearer $2" \
    -H 'Content-Type: application/json' -d '{}' -o "$WORKDIR/opt.json"
  jsonfield "$WORKDIR/opt.json" sessionId
}
finish_reg() { # <base> <token> <sessionId> -> prints body
  curl -s -X POST "$1/auth/webauthn/register/verify" -H "Authorization: Bearer $2" \
    -H 'Content-Type: application/json' -d "{\"sessionId\":\"$3\",\"response\":{\"id\":\"invalid\"}}"
}

ALL_HANDLES=()

note "1. Registration: begin on A, finish on B"
SID=$(begin_reg "$A" "$TOKEN"); ALL_HANDLES+=("$SID")
assert_contains "Invalid registration response" "$(finish_reg "$B" "$TOKEN" "$SID")" \
  "instance B consumed the session instance A created"

note "2. Login (discoverable): begin on B, finish on A"
curl -s -X POST "$B/auth/webauthn/login/options" -H 'Content-Type: application/json' \
  -d '{}' -o "$WORKDIR/lopt.json"
LSID=$(jsonfield "$WORKDIR/lopt.json" sessionId); ALL_HANDLES+=("$LSID")
assert_contains "Invalid login response" \
  "$(curl -s -X POST "$A/auth/webauthn/login/verify" -H 'Content-Type: application/json' \
      -d "{\"sessionId\":\"$LSID\",\"response\":{\"id\":\"invalid\"}}")" \
  "instance A consumed the login session instance B created"

note "3. Single-use across instances"
SID2=$(begin_reg "$A" "$TOKEN"); ALL_HANDLES+=("$SID2")
assert_contains "Invalid registration response" "$(finish_reg "$B" "$TOKEN" "$SID2")" \
  "first consume on B succeeds"
assert_contains "session expired" "$(finish_reg "$A" "$TOKEN" "$SID2")" \
  "replay on A is refused — consumption is global, not per-process"

note "4. Registration handle is scoped to its user"
OTHER=$(mkuser "mr-other-$STAMP@example.com")
SID3=$(begin_reg "$A" "$OTHER"); ALL_HANDLES+=("$SID3")
assert_contains "session expired" "$(finish_reg "$B" "$TOKEN" "$SID3")" \
  "another account cannot redeem the handle (uniform rejection)"

note "5. Expired handle refused while its row still exists"
SID4=$(begin_reg "$A" "$TOKEN"); ALL_HANDLES+=("$SID4")
HASH=$(python3 -c "import hashlib,sys;print(hashlib.sha256(sys.argv[1].encode()).hexdigest())" "$SID4")
psql -h "$PGHOST" -p "$PGPORT" -U "$PGUSER" -d "$PGDATABASE" -q \
  -c "UPDATE webauthn_sessions SET expires_at = NOW() - interval '1 minute' WHERE id_hash = decode('$HASH','hex');"
assert_contains "session expired" "$(finish_reg "$B" "$TOKEN" "$SID4")" \
  "expired handle refused"
present=$(psql -h "$PGHOST" -p "$PGPORT" -U "$PGUSER" -d "$PGDATABASE" -t -A \
  -c "SELECT count(*) FROM webauthn_sessions WHERE id_hash = decode('$HASH','hex');")
if [ "$present" = "1" ]; then
  ok "row still present — refusal came from the expires_at predicate, not cleanup"
else
  bad "expected the expired row to still be present, found $present"
fi

note "6. Ceremony survives the death of the process that began it"
SID5=$(begin_reg "$A" "$TOKEN"); ALL_HANDLES+=("$SID5")
for pid in "${PIDS[@]}"; do kill "$pid" 2>/dev/null || true; done
sleep 3
PIDS=()
start_instance "$PORT_C" "$WORKDIR/c.log" || exit 1
assert_contains "Invalid registration response" \
  "$(finish_reg "http://127.0.0.1:$PORT_C/api/v1" "$TOKEN" "$SID5")" \
  "a freshly started process completed a ceremony begun by a dead one"

note "7. No raw handle appears in any instance log"
leaked=0
for h in "${ALL_HANDLES[@]}"; do
  [ -z "$h" ] && continue
  if grep -qF -- "$h" "$WORKDIR"/*.log 2>/dev/null; then
    bad "handle leaked into logs: $h"; leaked=1
  fi
done
[ $leaked -eq 0 ] && ok "no raw handle found in ${#ALL_HANDLES[@]} issued handles"

note "Result"
printf '  %d passed, %d failed\n\n' "$pass" "$fail"
[ "$fail" -eq 0 ] || exit 1
