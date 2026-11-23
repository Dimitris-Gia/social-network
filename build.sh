#!/usr/bin/env bash
# Owner: A — bonus: build and start everything with one command (week 3)
set -euo pipefail
cd "$(dirname "$0")"
docker compose up --build -d
docker ps -a
