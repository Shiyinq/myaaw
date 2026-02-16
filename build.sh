#!/bin/bash


VERSION="${1:-v0.0.1}"
COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

echo "🚧 Building Myaaw CLI..."
echo "   Version: $VERSION"
echo "   Commit:  $COMMIT"
echo "   Date:    $DATE"


mkdir -p bin


go build -ldflags "-s -w -X main.Version=$VERSION -X main.Commit=$COMMIT -X main.Date=$DATE" -o bin/myaaw ./cmd/myaaw

if [ $? -eq 0 ]; then
    echo "✅ Build successful: ./bin/myaaw"
    echo ""
    ./bin/myaaw version
    echo ""
    echo "Try running:"
    echo "  ./bin/myaaw help"
else
    echo "❌ Build failed"
    exit 1
fi
