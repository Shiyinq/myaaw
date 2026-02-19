#!/bin/bash

# Kill by PID if file exists
if [ -f tmp/consumer-myaaw.pid ]; then
  pid=$(cat tmp/consumer-myaaw.pid)
  if ps -p $pid > /dev/null; then
    echo "Stopping consumer (PID: $pid)..."
    kill $pid
    # Wait for process to exit
    for i in {1..5}; do
      if ! ps -p $pid > /dev/null; then
        break
      fi
      sleep 1
    done
    # Force kill if still running
    if ps -p $pid > /dev/null; then
      echo "Force killing consumer..."
      kill -9 $pid
    fi
  else
    echo "Consumer PID $pid not found running."
  fi
  rm tmp/consumer-myaaw.pid
fi

# Fallback: Kill by name
echo "Cleaning up any remaining consumer processes..."
pkill -f "myaaw consumer" || true

echo "Consumer stopped."