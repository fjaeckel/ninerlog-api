#!/usr/bin/env bash
set -euo pipefail

# NinerLog API — Performance Test Runner
# Starts the performance stack, seeds data, runs all k6 scenarios, outputs summary.
#
# Prerequisites: docker, k6 (https://k6.io/docs/get-started/installation/)
#
# Usage:
#   ./scripts/run-perf-tests.sh              # Run all scenarios
#   ./scripts/run-perf-tests.sh --seed-only  # Only seed data
#   ./scripts/run-perf-tests.sh --skip-seed  # Skip seeding (use existing data)
#   ./scripts/run-perf-tests.sh auth         # Run specific scenario

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
PERF_DIR="$PROJECT_DIR/test/performance"
COMPOSE_FILE="$PROJECT_DIR/docker-compose.perf.yaml"

API_URL="http://localhost:3334"
PERF_API_URL="${PERF_API_URL:-$API_URL}"

SEED_ONLY=false
SKIP_SEED=false
SCENARIO_FILTER=""

# Parse arguments
while [[ $# -gt 0 ]]; do
  case $1 in
    --seed-only) SEED_ONLY=true; shift ;;
    --skip-seed) SKIP_SEED=true; shift ;;
    --keep|-k) KEEP_RUNNING=true; shift ;;
    *) SCENARIO_FILTER="$1"; shift ;;
  esac
done

KEEP_RUNNING=${KEEP_RUNNING:-false}

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m'

log() { echo -e "${CYAN}[perf]${NC} $1"; }
success() { echo -e "${GREEN}[perf]${NC} $1"; }
warn() { echo -e "${YELLOW}[perf]${NC} $1"; }
error() { echo -e "${RED}[perf]${NC} $1"; }

# Check for k6
if ! command -v k6 &>/dev/null; then
  error "k6 is not installed. Install it: brew install k6"
  exit 1
fi

# Cleanup function
cleanup() {
  if [[ "$KEEP_RUNNING" == "true" ]]; then
    warn "Keeping performance stack running (use 'docker compose -f $COMPOSE_FILE down -v' to stop)"
    return
  fi
  log "Stopping performance stack..."
  docker compose -f "$COMPOSE_FILE" down -v --remove-orphans 2>/dev/null || true
}
trap cleanup EXIT

# Start performance stack
log "Starting performance stack..."
docker compose -f "$COMPOSE_FILE" up -d --build

# Wait for API health
log "Waiting for API at $PERF_API_URL/health..."
RETRIES=0
MAX_RETRIES=60
until curl -sf "$PERF_API_URL/health" >/dev/null 2>&1; do
  RETRIES=$((RETRIES + 1))
  if [[ $RETRIES -ge $MAX_RETRIES ]]; then
    error "API failed to start after $MAX_RETRIES retries"
    docker compose -f "$COMPOSE_FILE" logs api
    exit 1
  fi
  sleep 2
done
success "API is healthy!"

# Seed data
if [[ "$SKIP_SEED" != "true" ]]; then
  log "Seeding performance data (100 users × 100 flights = 10,000 flights)..."
  PERF_API_URL="$PERF_API_URL" k6 run "$PERF_DIR/seed.js" --quiet 2>&1 | tail -20
  success "Seeding complete!"
fi

if [[ "$SEED_ONLY" == "true" ]]; then
  success "Seed-only mode. Stack is running at $PERF_API_URL"
  KEEP_RUNNING=true
  exit 0
fi

# Results directory
RESULTS_DIR="$PERF_DIR/results"
mkdir -p "$RESULTS_DIR"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)

# Run scenarios
SCENARIOS=("auth" "flights" "search" "dashboard" "exports" "spike")
FAILED=0

for scenario in "${SCENARIOS[@]}"; do
  if [[ -n "$SCENARIO_FILTER" && "$scenario" != "$SCENARIO_FILTER" ]]; then
    continue
  fi

  SCENARIO_FILE="$PERF_DIR/scenarios/$scenario.js"
  if [[ ! -f "$SCENARIO_FILE" ]]; then
    warn "Scenario not found: $scenario"
    continue
  fi

  echo ""
  log "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
  log "Running scenario: $scenario"
  log "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

  RESULT_FILE="$RESULTS_DIR/${scenario}_${TIMESTAMP}.json"

  if PERF_API_URL="$PERF_API_URL" k6 run \
    --out "json=$RESULT_FILE" \
    "$SCENARIO_FILE" 2>&1; then
    success "✅ $scenario passed"
  else
    error "❌ $scenario failed thresholds"
    FAILED=$((FAILED + 1))
  fi

  # Brief pause between scenarios
  sleep 2
done

# Summary
echo ""
log "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
if [[ $FAILED -eq 0 ]]; then
  success "All performance scenarios passed! 🎉"
  log "Results saved to: $RESULTS_DIR/"
else
  error "$FAILED scenario(s) failed performance thresholds"
  log "Results saved to: $RESULTS_DIR/"
  exit 1
fi
