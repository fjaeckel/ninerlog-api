#!/bin/bash

set -e

echo "🐳 Starting test database..."
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
