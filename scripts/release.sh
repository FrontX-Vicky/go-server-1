#!/usr/bin/env bash
set -euo pipefail

APP_ROOT=/var/www/go-workspace/server_1
LIVE=$APP_ROOT/live

mkdir -p "$LIVE"

# Build directly into the live folder with clear names
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o "$LIVE/app" ./cmd/api
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o "$LIVE/app-worker" ./cmd/worker

# Restart the renamed services
sudo systemctl restart go-server-1.service || true
sudo systemctl restart go-server-1-worker.service || true

echo "Built:"
ls -lh "$LIVE/app" "$LIVE/app-worker"
