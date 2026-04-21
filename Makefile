.PHONY: help generate test test-short test-integration test-e2e coverage lint fmt build run bench test-perf test-perf-seed profile profile-pprof profile-explain migrate-up migrate-down migrate-create sqlc-generate docker-up docker-down clean

# Default target
.DEFAULT_GOAL := help

# Variables
BINARY_NAME=ninerlog-api
BUILD_DIR=bin
COVERAGE_FILE=coverage.out
COVERAGE_HTML=coverage.html

help: ## Show this help message
	@echo "NinerLog API - Available commands:"
	@echo ""
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'
	@echo ""

generate: ## Generate Go types from OpenAPI spec
	@echo "🔄 Generating Go server types from OpenAPI spec..."
	@./scripts/generate-server-types.sh
	@go mod tidy
	@echo "✅ Generation complete"

test: ## Run unit tests only
	@echo "🧪 Running unit tests..."
	@go test -v -short -cover ./...

test-short: test ## Alias for test (unit tests only)

test-integration: ## Run integration tests (requires test DB)
	@echo "🧪 Running integration tests..."
	@docker compose -f docker-compose.test.yaml up -d
	@sleep 3
	@docker compose -f docker-compose.test.yaml exec -T postgres-test psql -U testuser -d ninerlog_test < db/migrations/test_init.sql 2>/dev/null || true
	@export TEST_DB_HOST=localhost TEST_DB_PORT=5433 TEST_DB_USER=testuser TEST_DB_PASSWORD=testpass TEST_DB_NAME=ninerlog_test && \
		go test -v ./internal/repository/postgres/... | grep -v "no test files"
	@docker compose -f docker-compose.test.yaml down

test-e2e: ## Run end-to-end tests (requires test DB)
	@echo "🧪 Running e2e tests..."
	@docker compose -f docker-compose.test.yaml up -d
	@sleep 3
	@docker compose -f docker-compose.test.yaml exec -T postgres-test psql -U testuser -d ninerlog_test < db/migrations/test_init.sql 2>/dev/null || true
	@export TEST_DB_HOST=localhost TEST_DB_PORT=5433 TEST_DB_USER=testuser TEST_DB_PASSWORD=testpass TEST_DB_NAME=ninerlog_test && \
		go test -v -tags=e2e ./test/e2e/...
	@docker compose -f docker-compose.test.yaml down

test-e2e-full: ## Run full e2e tests against real API (docker-compose)
	@echo "🧪 Running full e2e test suite..."
	@./scripts/run-e2e-tests.sh

test-all: ## Run all tests (unit, integration, e2e)
	@echo "🧪 Running all tests..."
	@./scripts/run-all-tests.sh

coverage: ## Generate HTML coverage report
	@echo "📊 Generating coverage report..."
	@go test -coverprofile=$(COVERAGE_FILE) ./...
	@go tool cover -html=$(COVERAGE_FILE) -o $(COVERAGE_HTML)
	@echo "✅ Coverage report: $(COVERAGE_HTML)"

lint: ## Lint code with golangci-lint
	@echo "🔍 Linting code..."
	@golangci-lint run

fmt: ## Format code with go fmt
	@echo "🎨 Formatting code..."
	@go fmt ./...

build: ## Build binary
	@echo "🔨 Building application..."
	@mkdir -p $(BUILD_DIR)
	@go build -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/api
	@echo "✅ Binary: $(BUILD_DIR)/$(BINARY_NAME)"

run: ## Run application
	@echo "🚀 Running application..."
	@go run ./cmd/api/main.go

bench: ## Run Go benchmark tests
	@echo "⏱  Running benchmark tests..."
	@go test -run='^$$' -bench=. -benchmem -count=3 ./internal/service/flightcalc/... ./internal/service/currency/... ./internal/service/...

test-perf: ## Run k6 performance tests (requires docker + k6)
	@echo "🔥 Running performance tests..."
	@./scripts/run-perf-tests.sh

test-perf-seed: ## Seed performance test data only
	@./scripts/run-perf-tests.sh --seed-only

profile: ## Run full profiling (pprof + EXPLAIN ANALYZE)
	@echo "🔬 Running profiling suite..."
	@./scripts/run-profiling.sh all

profile-pprof: ## Collect pprof profiles under load
	@echo "🔬 Collecting pprof profiles..."
	@./scripts/run-profiling.sh pprof

profile-explain: ## Run EXPLAIN ANALYZE queries
	@echo "🔬 Running EXPLAIN ANALYZE..."
	@./scripts/run-profiling.sh explain

migrate-up: ## Apply database migrations
	@echo "⬆️  Running database migrations..."
	@migrate -path db/migrations -database "$(DATABASE_URL)" up

migrate-down: ## Rollback last migration
	@echo "⬇️  Rolling back database migration..."
	@migrate -path db/migrations -database "$(DATABASE_URL)" down 1

migrate-create: ## Create new migration (usage: make migrate-create NAME=create_users_table)
	@if [ -z "$(NAME)" ]; then \
		echo "Usage: make migrate-create NAME=<migration_name>"; \
		exit 1; \
	fi
	@echo "📝 Creating migration: $(NAME)"
	@migrate create -ext sql -dir db/migrations -seq $(NAME)

sqlc-generate: ## Generate sqlc code
	@echo "⚙️  Generating sqlc code..."
	@sqlc generate

docker-build: ## Build Docker image
	@echo "🐳 Building Docker image..."
	@docker build -t $(BINARY_NAME):latest .

docker-up: ## Start Docker containers
	@echo "🐳 Starting Docker containers..."
	@docker-compose up -d

docker-down: ## Stop Docker containers
	@echo "🐳 Stopping Docker containers..."
	@docker-compose down

docker-logs: ## View Docker logs
	@docker-compose logs -f

clean: ## Clean build artifacts and test files
	@echo "🧹 Cleaning..."
	@rm -rf $(BUILD_DIR)
	@rm -f $(COVERAGE_FILE) $(COVERAGE_HTML)
	@echo "✅ Clean complete"
