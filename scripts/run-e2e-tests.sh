#!/usr/bin/env bash
# Run end-to-end tests against a real API server.
# This script starts docker-compose, waits for the API, runs tests, and cleans up.
#
# Usage:
#   bash scripts/run-e2e-tests.sh                    # run all tests
#   bash scripts/run-e2e-tests.sh TestEASA            # run tests matching "TestEASA"
#   bash scripts/run-e2e-tests.sh "TestFAA_IR|TestEASA_IR"  # run multiple patterns
#   bash scripts/run-e2e-tests.sh TestGermanUL -k     # keep environment running after tests
#
# Options:
#   -k, --keep    Keep the e2e environment running after tests (skip cleanup).
#                 Useful for re-running tests quickly without rebuilding.
#
# Re-run against an already-running environment (after -k):
#   E2E_API_URL=http://localhost:3333 go test -v -tags=e2e -count=1 -timeout=300s -run "TestEASA_IR" ./test/e2e/...
#
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR/.."

# Parse arguments
TEST_FILTER=""
KEEP_ENV=false
for arg in "$@"; do
    case "$arg" in
        -k|--keep) KEEP_ENV=true ;;
        *) TEST_FILTER="$arg" ;;
    esac
done

COMPOSE_FILE="docker-compose.e2e.yaml"
API_URL="${E2E_API_URL:-http://localhost:3333}"
MAILPIT_URL="${E2E_MAILPIT_URL:-http://localhost:8025}"

cleanup() {
    if [ "$KEEP_ENV" = true ]; then
        echo "🔧 Keeping e2e environment running (-k flag). To tear down manually:"
        echo "   docker compose -f $COMPOSE_FILE down -v --remove-orphans"
        echo ""
        echo "💡 Re-run tests quickly with:"
        echo "   E2E_API_URL=$API_URL go test -v -tags=e2e -count=1 -timeout=300s -run \"PATTERN\" ./test/e2e/..."
        return
    fi
    echo "🧹 Tearing down e2e environment..."
    docker compose -f "$COMPOSE_FILE" down -v --remove-orphans 2>/dev/null || true
}

# Always cleanup on exit
trap cleanup EXIT

echo "🚀 Starting e2e test environment..."
docker compose -f "$COMPOSE_FILE" down -v --remove-orphans 2>/dev/null || true
docker compose -f "$COMPOSE_FILE" up --build -d

echo "⏳ Waiting for API to be ready at $API_URL..."
MAX_RETRIES=60
RETRY=0
until curl -sf "$API_URL/health" > /dev/null 2>&1; do
    RETRY=$((RETRY + 1))
    if [ $RETRY -ge $MAX_RETRIES ]; then
        echo "❌ API failed to start after $MAX_RETRIES attempts"
        echo "📋 API logs:"
        docker compose -f "$COMPOSE_FILE" logs api
        exit 1
    fi
    sleep 2
done
echo "✅ API is ready"

echo "⏳ Waiting for MailPit to be ready at $MAILPIT_URL..."
RETRY=0
until curl -sf "$MAILPIT_URL/api/v1/info" > /dev/null 2>&1; do
    RETRY=$((RETRY + 1))
    if [ $RETRY -ge 30 ]; then
        echo "❌ MailPit failed to start after 30 attempts"
        exit 1
    fi
    sleep 1
done
echo "✅ MailPit is ready"

echo "🧪 Running e2e tests..."
RUN_FLAG=""
if [ -n "$TEST_FILTER" ]; then
    RUN_FLAG="-run $TEST_FILTER"
    echo "   Filter: $TEST_FILTER"
fi
E2E_API_URL="$API_URL" E2E_MAILPIT_URL="$MAILPIT_URL" go test -v -tags=e2e -count=1 -timeout=300s $RUN_FLAG ./test/e2e/...
TEST_EXIT=$?

if [ $TEST_EXIT -eq 0 ]; then
    echo "✅ All e2e tests passed"
else
    echo "❌ Some e2e tests failed"
    echo ""
    echo "📋 API logs (last 50 lines):"
    docker compose -f "$COMPOSE_FILE" logs --tail=50 api
fi

exit $TEST_EXIT
