#!/bin/bash
set -e

# Generate Go server types from OpenAPI spec
# Usage: ./scripts/generate-server-types.sh [path-to-openapi.yaml]

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# Default to project repo spec
OPENAPI_SPEC="${1:-../pilotlog-project/api-spec/openapi.yaml}"

# Check if spec exists
if [ ! -f "$OPENAPI_SPEC" ]; then
    echo "❌ OpenAPI spec not found at: $OPENAPI_SPEC"
    exit 1
fi

echo "🔍 Using OpenAPI spec: $OPENAPI_SPEC"

# Check if oapi-codegen is installed
if ! command -v oapi-codegen &> /dev/null; then
    echo "📦 Installing oapi-codegen..."
    go install github.com/deepmap/oapi-codegen/v2/cmd/oapi-codegen@latest
fi

# Output directory
OUTPUT_DIR="$PROJECT_ROOT/internal/api/generated"
mkdir -p "$OUTPUT_DIR"

echo "🧹 Cleaning output directory..."
rm -f "$OUTPUT_DIR"/*.go

echo "⚙️  Generating Go types..."
oapi-codegen -package generated -generate types \
    -o "$OUTPUT_DIR/types.go" \
    "$OPENAPI_SPEC"

echo "⚙️  Generating Gin server interface..."
oapi-codegen -package generated -generate gin \
    -o "$OUTPUT_DIR/server.go" \
    "$OPENAPI_SPEC"

echo "⚙️  Generating request/response helpers..."
oapi-codegen -package generated -generate spec \
    -o "$OUTPUT_DIR/spec.go" \
    "$OPENAPI_SPEC"

echo "📝 Adding package documentation..."
cat > "$OUTPUT_DIR/doc.go" << 'EOF'
// Package generated contains auto-generated code from the OpenAPI specification.
//
// ⚠️ DO NOT EDIT THESE FILES MANUALLY
//
// This package is automatically generated from the OpenAPI spec.
// To regenerate after spec changes, run:
//
//	go generate ./...
//
// Or manually:
//
//	./scripts/generate-server-types.sh
//
// Source: pilotlog-project/api-spec/openapi.yaml
// Generator: oapi-codegen v2
package generated
EOF

echo "📝 Creating go:generate directive..."
cat > "$OUTPUT_DIR/generate.go" << EOF
package generated

//go:generate bash ../../scripts/generate-server-types.sh
EOF

echo "🎨 Formatting generated code..."
go fmt "$OUTPUT_DIR"/*.go

echo "✅ Go server types generated successfully in $OUTPUT_DIR"
echo ""
echo "Generated files:"
echo "  - types.go     (OpenAPI schemas as Go structs)"
echo "  - server.go    (Gin handler interfaces)"
echo "  - spec.go      (OpenAPI spec embedded)"
echo "  - doc.go       (Package documentation)"
echo ""
echo "Next steps:"
echo "  1. Implement ServerInterface in your handlers"
echo "  2. Register handlers with Gin router"
echo "  3. Run tests: go test ./..."
echo "  4. Commit changes"
