#!/usr/bin/env bash
set -euo pipefail

echo "Running rims-go service..."
go run ./cmd/server
