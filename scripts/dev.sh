#!/bin/bash
# Development script - rebuilds and runs Airbridge
set -e

cd "$(dirname "$0")/.."

echo "Building airbridge..."
make build

echo "Starting airbridge..."
exec ./bin/airbridge "$@"
