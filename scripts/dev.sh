#!/bin/bash
# Development script - rebuilds and runs Airbridge
set -e

cd "$(dirname "$0")/.."

# Log file location (truncated on each restart to prevent growth)
LOG_FILE="${AIRBRIDGE_LOG_FILE:-/tmp/airbridge-dev.log}"

echo "Building airbridge..."
make build

echo "Starting airbridge..."
echo "Logs: $LOG_FILE (tail -f $LOG_FILE)"

# Truncate log file on restart
> "$LOG_FILE"

# Run with output to both terminal and log file
exec ./bin/airbridge "$@" 2>&1 | tee "$LOG_FILE"
