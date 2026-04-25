#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

SKIP_DOCKER=0
SKIP_FRONTEND=0
INSTALL=0

for arg in "$@"; do
  case "$arg" in
    --skip-docker) SKIP_DOCKER=1 ;;
    --skip-frontend) SKIP_FRONTEND=1 ;;
    --install) INSTALL=1 ;;
    *)
      echo "Unknown argument: $arg"
      echo "Usage: scripts/start-dev.sh [--skip-docker] [--skip-frontend] [--install]"
      exit 1
      ;;
  esac
done

ensure_file_from_example() {
  local path="$1"
  local example="$2"
  if [[ ! -f "$path" && -f "$example" ]]; then
    cp "$example" "$path"
    echo "Created $path from $example. Please fill required API keys if needed."
  fi
}

wait_port() {
  local host="$1"
  local port="$2"
  local timeout="${3:-45}"
  local start
  start="$(date +%s)"
  while (( "$(date +%s)" - start < timeout )); do
    if command -v nc >/dev/null 2>&1; then
      nc -z "$host" "$port" >/dev/null 2>&1 && return 0
    else
      (echo >"/dev/tcp/$host/$port") >/dev/null 2>&1 && return 0 || true
    fi
    sleep 1
  done
  return 1
}

cleanup() {
  echo
  echo "Stopping NotebookMind dev processes..."
  for pid in ${API_PID:-} ${WORKER_PID:-} ${WEB_PID:-}; do
    if [[ -n "$pid" ]] && kill -0 "$pid" >/dev/null 2>&1; then
      kill "$pid" >/dev/null 2>&1 || true
    fi
  done
}
trap cleanup INT TERM EXIT

ensure_file_from_example ".env" ".env.example"
ensure_file_from_example "web/.env.local" "web/.env.example"

mkdir -p logs tmp/uploads storage/exports

if [[ "$SKIP_DOCKER" -eq 0 ]]; then
  echo "Starting PostgreSQL and Redis..."
  docker compose up -d postgres redis
fi

if [[ "$INSTALL" -eq 1 ]]; then
  echo "Installing Python export dependencies..."
  python3 -m pip install -r scripts/requirements-export.txt
fi

if [[ "$SKIP_FRONTEND" -eq 0 && ! -d web/node_modules ]]; then
  echo "Installing frontend dependencies..."
  (cd web && npm install)
fi

wait_port 127.0.0.1 5432 45 || echo "Warning: PostgreSQL port 5432 is not reachable yet."
wait_port 127.0.0.1 6380 45 || echo "Warning: Redis port 6380 is not reachable yet."

echo "Starting API on http://localhost:8081 ..."
go run ./cmd/api > logs/dev-api.log 2>&1 &
API_PID=$!

sleep 8

echo "Starting worker..."
go run ./cmd/worker > logs/dev-worker.log 2>&1 &
WORKER_PID=$!

if [[ "$SKIP_FRONTEND" -eq 0 ]]; then
  echo "Starting frontend on http://localhost:3000 ..."
  (cd web && NEXT_PUBLIC_API_URL=http://localhost:8081/api/v1 npm run dev) > logs/dev-web.log 2>&1 &
  WEB_PID=$!
fi

echo
echo "NotebookMind dev stack is running:"
echo "  API:      http://localhost:8081/api/v1"
echo "  Frontend: http://localhost:3000"
echo "  Postgres: localhost:5432"
echo "  Redis:    localhost:6380"
echo
echo "Logs:"
echo "  logs/dev-api.log"
echo "  logs/dev-worker.log"
echo "  logs/dev-web.log"
echo
echo "Press Ctrl+C to stop API/worker/frontend."

wait
