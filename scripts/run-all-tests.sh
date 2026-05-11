#!/bin/bash

set -e

echo "�️  Checking that flight-rule logic is not duplicated outside flightrules/…"
# Any new fallback chain "PICName → InstructorName" or inline FSTD/remarks
# concatenation in handlers/ or repository/ must instead use the centralised
# helpers in internal/service/flightrules.
violations=$(
  {
    grep -RnE 'PICName[^|]*InstructorName|InstructorName[^|]*PICName' internal/api internal/repository 2>/dev/null || true
    grep -RnE 'remarks \+= "[[:space:]]*\[(IPC|FR|PC)\]"' internal/api internal/repository 2>/dev/null || true
    grep -RnE 'f\.FSTDType != nil && \*f\.FSTDType != "" && f\.SimulatedFlightTime > 0' internal/api internal/repository 2>/dev/null || true
  } | grep -v 'flightrules' || true
)
if [ -n "$violations" ]; then
  echo "❌ Flight-rule duplication detected — use internal/service/flightrules helpers:"
  echo "$violations"
  exit 1
fi

echo "�🐳 Starting test database..."
docker compose -f docker-compose.test.yaml up -d

echo "⏳ Waiting for database to be ready..."
sleep 3

echo "📦 Running database migrations..."
docker compose -f docker-compose.test.yaml exec -T postgres-test psql -U testuser -d ninerlog_test < db/migrations/test_init.sql || echo "⚠️  Migrations might need manual setup"

echo "🧪 Running unit tests..."
go test -v -short ./...

echo ""
echo "🧪 Running integration tests..."
go test -v -run Integration ./internal/repository/postgres/...

echo ""
echo "🧪 Running e2e tests..."
go test -v -tags=e2e ./test/e2e/...

echo ""
echo "🛑 Stopping test database..."
docker compose -f docker-compose.test.yaml down

echo ""
echo "✅ All tests completed!"
