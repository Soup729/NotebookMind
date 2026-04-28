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

run_checked() {
  "$@"
}

docker_container_using_port() {
  local port="$1"
  docker ps --format '{{.Names}}|{{.Ports}}' 2>/dev/null | awk -F'|' -v port="$port" '$2 ~ ":" port "->" || $2 ~ "\\." port "->" { print $1; exit }'
}

assert_docker_port_available() {
  local port="$1"
  shift
  local allowed="$*"
  local container
  container="$(docker_container_using_port "$port" || true)"
  if [[ -n "$container" && " $allowed " != *" $container "* ]]; then
    echo "Port $port is already used by Docker container '$container'. Stop it first, for example: docker stop $container" >&2
    exit 1
  fi
}

stop_docker_container_using_port() {
  local port="$1"
  shift
  local allowed="$*"
  local container
  container="$(docker_container_using_port "$port" || true)"
  if [[ -n "$container" && " $allowed " != *" $container "* ]]; then
    echo "Stopping Docker container '$container' using port $port..."
    run_checked docker stop "$container"
  fi
}

stop_local_port_owner() {
  local port="$1"
  local name="$2"
  local pids=""
  if command -v lsof >/dev/null 2>&1; then
    pids="$(lsof -tiTCP:"$port" -sTCP:LISTEN 2>/dev/null || true)"
  elif command -v fuser >/dev/null 2>&1; then
    pids="$(fuser "$port/tcp" 2>/dev/null || true)"
  fi
  if [[ -z "$pids" ]]; then
    return
  fi
  echo "Stopping $name port $port owner(s): $pids..."
  # shellcheck disable=SC2086
  kill $pids >/dev/null 2>&1 || true
  sleep 1
  # shellcheck disable=SC2086
  kill -9 $pids >/dev/null 2>&1 || true
}

stop_compose_service_if_running() {
  local service="$1"
  if docker compose ps -q "$service" >/dev/null 2>&1 && [[ -n "$(docker compose ps -q "$service" 2>/dev/null)" ]]; then
    echo "Stopping existing Docker service '$service'..."
    run_checked docker compose stop "$service"
  fi
}

assert_local_port_available() {
  local port="$1"
  local name="$2"
  if command -v lsof >/dev/null 2>&1; then
    local owner
    owner="$(lsof -nP -iTCP:"$port" -sTCP:LISTEN 2>/dev/null | awk 'NR==2 {print $1 " pid=" $2}')"
    if [[ -n "$owner" ]]; then
      echo "$name port $port is already in use by $owner. Stop it first or use the Docker stack only." >&2
      exit 1
    fi
  elif command -v nc >/dev/null 2>&1 && nc -z 127.0.0.1 "$port" >/dev/null 2>&1; then
    echo "$name port $port is already in use. Stop the existing process first or use the Docker stack only." >&2
    exit 1
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
  run_checked docker version >/dev/null
  stop_compose_service_if_running worker
  stop_docker_container_using_port 5432 notebookmind_postgres
  stop_docker_container_using_port 6380 notebookmind_redis
  stop_docker_container_using_port 8081
  if [[ "$SKIP_FRONTEND" -eq 0 ]]; then
    stop_docker_container_using_port 3000
  fi
  echo "Starting PostgreSQL and Redis..."
  run_checked docker compose up -d postgres redis
fi

if [[ "$INSTALL" -eq 1 ]]; then
  echo "Installing Python export dependencies..."
  run_checked python3 -m pip install -r scripts/requirements-export.txt
fi

if [[ "$SKIP_FRONTEND" -eq 0 && ! -d web/node_modules ]]; then
  echo "Installing frontend dependencies..."
  if [[ -f web/package-lock.json ]]; then
    (cd web && run_checked npm ci)
  else
    (cd web && run_checked npm install)
  fi
fi

wait_port 127.0.0.1 5432 45 || { echo "PostgreSQL port 5432 is not reachable. Check Docker and run: docker compose logs postgres" >&2; exit 1; }
wait_port 127.0.0.1 6380 45 || { echo "Redis port 6380 is not reachable. Check Docker and run: docker compose logs redis" >&2; exit 1; }

stop_local_port_owner 8081 API
if [[ "$SKIP_FRONTEND" -eq 0 ]]; then
  stop_local_port_owner 3000 Frontend
fi
assert_local_port_available 8081 API
if [[ "$SKIP_FRONTEND" -eq 0 ]]; then
  assert_local_port_available 3000 Frontend
fi

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
