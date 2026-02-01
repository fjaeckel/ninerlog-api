.PHONY: help generate test test-short test-integration test-e2e coverage lint fmt build run migrate-up migrate-down migrate-create sqlc-generate docker-up docker-down clean

# Default target
.DEFAULT_GOAL := help

# Variables
BINARY_NAME=pilotlog-api
BUILD_DIR=bin
COVERAGE_FILE=coverage.out
COVERAGE_HTML=coverage.html

help: ## Show this help message
	@echo "PilotLog API - Available commands:"
	@echo ""
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'
	@echo ""

generate: ## Generate Go types from OpenAPI spec
	@echo "🔄 Generating Go server types from OpenAPI spec..."
	@./scripts/generate-server-types.sh
	@go mod tidy
	@echo "✅ Generation complete"

test: ## Run all tests with coverage
	@echo "🧪 Running tests..."
	@go test -v -cover ./...

test-short: ## Run unit tests only
	@echo "🧪 Running unit tests only..."
	@go test -v -short ./...

test-integration: ## Run integration tests
	@echo "🧪 Running integration tests..."
	@go test -v -run Integration ./...

test-e2e: ## Run end-to-end tests
	@echo "🧪 Running e2e tests..."
	@go test -v -tags=e2e ./...

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
