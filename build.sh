#!/bin/bash


VERSION="${1:-v0.0.1}"
COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
PLATFORMS=("darwin/amd64" "darwin/arm64" "linux/amd64" "windows/amd64")

echo "🚧 Building Myaaw CLI for multiple platforms..."
echo "   Version: $VERSION"
echo "   Commit:  $COMMIT"
echo "   Date:    $DATE"



echo "🔨 Building local binary for development..."
go build -ldflags "-s -w -X main.Version=$VERSION -X main.Commit=$COMMIT -X main.Date=$DATE" -o bin/myaaw ./cmd/myaaw
if [ $? -eq 0 ]; then
    echo "✅ Local build: ./bin/myaaw"
else
    echo "❌ Local build failed"
    exit 1
fi

for PLATFORM in "${PLATFORMS[@]}"; do
    OS=${PLATFORM%/*}
    ARCH=${PLATFORM#*/}
    
    # We create a specific folder for each for cleaner distribution
    mkdir -p "bin/$OS-$ARCH"
    
    OUTPUT="bin/$OS-$ARCH/myaaw"
    if [ "$OS" == "windows" ]; then
        OUTPUT+=".exe"
    fi

    echo "📦 Building release for $OS/$ARCH..."
    GOOS=$OS GOARCH=$ARCH go build -ldflags "-s -w -X main.Version=$VERSION -X main.Commit=$COMMIT -X main.Date=$DATE" -o "$OUTPUT" ./cmd/myaaw
    
    if [ $? -ne 0 ]; then
        echo "❌ Failed to build $OS/$ARCH"
        exit 1
    fi
done

echo ""
echo "✅ Build complete! release binaries are in bin/[platform]/myaaw"
ls -R bin/


