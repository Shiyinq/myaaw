#!/bin/bash
# Development helper: build the Myaaw binary once, then run the consumer for
# one agent. Usage: ./start-consumer-myaaw.sh [agent]
set -euo pipefail

cd "$(dirname "$0")"

AGENT="${1:-default}"
if ! [[ "$AGENT" =~ ^[A-Za-z0-9][A-Za-z0-9_-]*$ ]]; then
  echo "Invalid agent name: $AGENT"
  echo "Use only letters, numbers, '_' and '-', starting with a letter or number."
  exit 1
fi

mkdir -p tmp

PID_FILE="tmp/consumer-myaaw-${AGENT}.pid"
LOG_FILE="tmp/consumer-myaaw-${AGENT}.log"

# Stop the previous consumer for this agent only.
if [ -f "$PID_FILE" ]; then
  OLD_PID="$(cat "$PID_FILE")"
  pkill -P "$OLD_PID" 2>/dev/null || true
  kill "$OLD_PID" 2>/dev/null || true
  rm -f "$PID_FILE"
fi
pkill -f "myaaw --agent ${AGENT} consumer run" 2>/dev/null || true

# Build once; all agents share the same development binary.
echo "Building myaaw development binary..."
go build -o tmp/myaaw ./cmd/myaaw

echo "Starting consumer for agent ${AGENT}..."
./tmp/myaaw --agent "$AGENT" consumer run > "$LOG_FILE" 2>&1 &
echo $! > "$PID_FILE"
echo "Consumer agent ${AGENT} started with PID $(cat "$PID_FILE")"
echo "Logs: $LOG_FILE"
