#!/usr/bin/env bash
# Verify every module linked into the binary carries a licence compatible with
# the AGPL-3.0-only, and optionally export the third-party notices.
#
# Usage:
#   ./scripts/check-dependency-licenses.sh               check only, exit 1 on failure
#   ./scripts/check-dependency-licenses.sh --save DIR    check, then write every
#                                                        dependency's licence file
#                                                        under DIR (shipped in the
#                                                        Docker image as /app/licenses)
#
# Policy and rationale: docs/LICENSING.md.
set -euo pipefail

GO_LICENSES_VERSION="v1.6.0"
MODULE="github.com/fjaeckel/ninerlog-api"

# Modules whose licence file the classifier cannot read, each reviewed by hand.
# Test-only; its LICENSE is BSD-3-Clause with a non-standard preamble.
REVIEWED_UNCLASSIFIABLE="github.com/DATA-DOG/go-sqlmock"

# SPDX identifiers as reported by go-licenses. Every entry is compatible with
# the AGPL-3.0-only; anything else, including "Unknown", fails the check.
ALLOWED_LICENSES="Apache-2.0,BSD-2-Clause,BSD-3-Clause,ISC,MIT,MPL-2.0,0BSD,Unlicense,CC0-1.0,GPL-3.0,LGPL-2.1,LGPL-3.0,AGPL-3.0"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$PROJECT_ROOT"

SAVE_DIR=""
case "${1:-}" in
    "") ;;
    --save)
        SAVE_DIR="${2:-}"
        [ -n "$SAVE_DIR" ] || { echo "usage: $0 [--save DIR]" >&2; exit 2; }
        ;;
    *) echo "usage: $0 [--save DIR]" >&2; exit 2 ;;
esac

export PATH="$PATH:$(go env GOPATH)/bin"
# go-licenses tells the standard library apart from modules by GOROOT.
export GOROOT="$(go env GOROOT)"

installed="$(go version -m "$(command -v go-licenses 2>/dev/null || echo /nonexistent)" 2>/dev/null \
    | awk '$1 == "mod" && $2 == "github.com/google/go-licenses" { print $3 }')"
if [ "$installed" != "$GO_LICENSES_VERSION" ]; then
    echo "Installing go-licenses $GO_LICENSES_VERSION (found: ${installed:-none})..."
    go install "github.com/google/go-licenses@$GO_LICENSES_VERSION"
fi

LOG="$(mktemp)"
trap 'rm -f "$LOG"' EXIT

# Warnings about assembly files are noise; keep everything else.
quiet() { grep -vE '^W[0-9]{4} |\.(s|h)$' "$LOG" || true; }

echo "Checking dependency licences against the allow-list..."
if ! go-licenses check ./... --include_tests --ignore "$MODULE" --ignore "$REVIEWED_UNCLASSIFIABLE" \
        --allowed_licenses="$ALLOWED_LICENSES" >"$LOG" 2>&1; then
    echo "FAIL: a dependency carries a licence outside the allow-list."
    quiet
    echo
    echo "Allowed: $ALLOWED_LICENSES"
    echo "See docs/LICENSING.md before adding a dependency with another licence."
    exit 1
fi

count="$(go-licenses report ./cmd/api --ignore "$MODULE" 2>/dev/null | grep -c . || true)"
echo "OK: $count module(s) linked into the binary carry an allow-listed licence."

if [ -n "$SAVE_DIR" ]; then
    echo "Exporting third-party licence notices to $SAVE_DIR..."
    rm -rf "$SAVE_DIR"
    if ! go-licenses save ./cmd/api --ignore "$MODULE" --save_path="$SAVE_DIR" >"$LOG" 2>&1; then
        echo "FAIL: could not export licence notices."
        quiet
        exit 1
    fi
    echo "OK: $(find "$SAVE_DIR" -type f | wc -l | tr -d ' ') notice file(s) written."
fi
