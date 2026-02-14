#!/bin/bash
cd "$(dirname "$0")"

mkdir -p tmp

# Kill previous PID if exists
if [ -f tmp/consumer-myaaw.pid ]; then
  pkill -P $(cat tmp/consumer-myaaw.pid) || true
  kill $(cat tmp/consumer-myaaw.pid) || true
  rm tmp/consumer-myaaw.pid
fi

# Fallback cleanup by name
pkill -f "consumer-myaaw" || true
pkill -f "go run ./cmd/consumer" || true
# Be careful with 'main' as it's common, but maybe necessary for stuck go run children
# pkill -f "main" || true 

# Build binary first to avoid 'go run' child process issues
echo "Building consumer..."
go build -o tmp/consumer-myaaw ./cmd/consumer

if [ $? -ne 0 ]; then
    echo "Build failed! Check logs."
    exit 1
fi

echo "Starting consumer binary..."
./tmp/consumer-myaaw > tmp/consumer-myaaw.log 2>&1 &
echo $! > tmp/consumer-myaaw.pid
echo "Consumer started with PID $(cat tmp/consumer-myaaw.pid)"