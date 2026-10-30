#!/bin/bash
# Development helper: stop the consumer for one agent.
# Usage: ./stop-consumer-myaaw.sh [agent]
set -euo pipefail

cd "$(dirname "$0")"

AGENT="${1:-default}"
if ! [[ "$AGENT" =~ ^[A-Za-z0-9][A-Za-z0-9_-]*$ ]]; then
  echo "Invalid agent name: $AGENT"
  echo "Use only letters, numbers, '_' and '-', starting with a letter or number."
  exit 1
fi

PID_FILE="tmp/consumer-myaaw-${AGENT}.pid"

if [ -f "$PID_FILE" ]; then
  PID="$(cat "$PID_FILE")"
  if ps -p "$PID" > /dev/null 2>&1; then
    echo "Stopping consumer for agent ${AGENT} (PID: $PID)..."
    kill "$PID" 2>/dev/null || true
    for _ in 1 2 3 4 5; do
      if ! ps -p "$PID" > /dev/null 2>&1; then
        break
      fi
      sleep 1
    done
    if ps -p "$PID" > /dev/null 2>&1; then
      echo "Force killing consumer..."
      kill -9 "$PID" 2>/dev/null || true
    fi
  else
    echo "Consumer PID $PID for agent ${AGENT} is not running."
  fi
  rm -f "$PID_FILE"
else
  echo "No PID file found for agent ${AGENT}."
fi

# Fallback cleanup for this agent only.
pkill -f "myaaw --agent ${AGENT} consumer run" 2>/dev/null || true

echo "Consumer agent ${AGENT} stopped."
