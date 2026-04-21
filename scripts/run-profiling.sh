#!/usr/bin/env bash
# scripts/run-profiling.sh — Collect pprof and EXPLAIN ANALYZE profiles
#
# Prerequisites:
#   - docker-compose.perf.yaml stack running (with PPROF_ENABLED=true)
#   - Data seeded via: PERF_API_URL=http://localhost:3334 k6 run test/performance/seed.js
#   - psql available (brew install postgresql)
#   - go tool pprof available (comes with Go)
#
# Usage:
#   ./scripts/run-profiling.sh [pprof|explain|all]

set -euo pipefail

API_URL="${PERF_API_URL:-http://localhost:3334}"
PPROF_URL="${PPROF_URL:-http://localhost:6060}"
DB_HOST="${DB_HOST:-localhost}"
DB_PORT="${DB_PORT:-5435}"
DB_USER="${DB_USER:-perfuser}"
DB_NAME="${DB_NAME:-ninerlog_perf}"

RESULTS_DIR="test/performance/results"
mkdir -p "$RESULTS_DIR"

MODE="${1:-all}"

run_pprof() {
    echo "🔍 Collecting pprof profiles..."
    echo "   API: $API_URL"
    echo "   pprof: $PPROF_URL"
    echo ""

    # Verify pprof is available
    if ! curl -sf "$PPROF_URL/debug/pprof/" > /dev/null 2>&1; then
        echo "❌ pprof not available at $PPROF_URL/debug/pprof/"
        echo "   Make sure PPROF_ENABLED=true in docker-compose.perf.yaml"
        exit 1
    fi

    # Login and get a token for load generation
    TOKEN=$(curl -sf "$API_URL/api/v1/auth/login" \
        -H 'Content-Type: application/json' \
        -d '{"email":"perfuser-0000@ninerlog-perf.com","password":"PerfTest123!Secure"}' \
        | python3 -c "import sys,json; print(json.load(sys.stdin)['accessToken'])")

    echo "1️⃣  Heap profile (before load)..."
    curl -sf "$PPROF_URL/debug/pprof/heap" > "$RESULTS_DIR/heap-before.prof"

    echo "2️⃣  Generating load (flight list + dashboard + exports)..."
    for i in $(seq 1 50); do
        curl -sf "$API_URL/api/v1/flights?page=1&pageSize=25" \
            -H "Authorization: Bearer $TOKEN" > /dev/null &
        curl -sf "$API_URL/api/v1/users/me/statistics" \
            -H "Authorization: Bearer $TOKEN" > /dev/null &
        curl -sf "$API_URL/api/v1/reports/trends?months=12" \
            -H "Authorization: Bearer $TOKEN" > /dev/null &
    done
    wait

    echo "3️⃣  30-second CPU profile (during load)..."
    # Start load in background during CPU profiling
    for j in $(seq 1 3); do
        (
            for i in $(seq 1 100); do
                curl -sf "$API_URL/api/v1/flights?page=1&pageSize=25" \
                    -H "Authorization: Bearer $TOKEN" > /dev/null 2>&1
                curl -sf "$API_URL/api/v1/exports/csv" \
                    -H "Authorization: Bearer $TOKEN" > /dev/null 2>&1
            done
        ) &
    done
    curl -sf "$PPROF_URL/debug/pprof/profile?seconds=30" > "$RESULTS_DIR/cpu.prof"
    wait

    echo "4️⃣  Heap profile (after load)..."
    curl -sf "$PPROF_URL/debug/pprof/heap" > "$RESULTS_DIR/heap-after.prof"

    echo "5️⃣  Allocs profile..."
    curl -sf "$PPROF_URL/debug/pprof/allocs" > "$RESULTS_DIR/allocs.prof"

    echo "6️⃣  Goroutine profile..."
    curl -sf "$PPROF_URL/debug/pprof/goroutine" > "$RESULTS_DIR/goroutine.prof"

    echo ""
    echo "📊 Profile files saved to $RESULTS_DIR/"
    echo ""
    echo "Analyze with:"
    echo "  go tool pprof -http=:8080 $RESULTS_DIR/cpu.prof"
    echo "  go tool pprof -http=:8080 $RESULTS_DIR/heap-after.prof"
    echo "  go tool pprof -http=:8080 $RESULTS_DIR/allocs.prof"
    echo ""

    # Print top allocators from heap
    echo "📋 Top 10 heap allocators (after load):"
    go tool pprof -top -nodecount=10 "$RESULTS_DIR/heap-after.prof" 2>/dev/null || echo "  (install Go to view profiles)"
    echo ""
}

run_explain() {
    echo "🔍 Running EXPLAIN ANALYZE queries..."
    echo "   DB: $DB_HOST:$DB_PORT/$DB_NAME"
    echo ""

    PGPASSWORD=perfpass psql \
        -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$DB_NAME" \
        -f test/performance/explain_analyze.sql \
        2>&1 | tee "$RESULTS_DIR/explain_analyze.txt"

    echo ""
    echo "📊 EXPLAIN ANALYZE results saved to $RESULTS_DIR/explain_analyze.txt"
}

case "$MODE" in
    pprof)
        run_pprof
        ;;
    explain)
        run_explain
        ;;
    all)
        run_explain
        echo ""
        echo "=========================================="
        echo ""
        run_pprof
        ;;
    *)
        echo "Usage: $0 [pprof|explain|all]"
        exit 1
        ;;
esac
