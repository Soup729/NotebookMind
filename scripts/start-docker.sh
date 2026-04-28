#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

BUILD=0
DETACHED=1

for arg in "$@"; do
  case "$arg" in
    --build) BUILD=1 ;;
    --foreground) DETACHED=0 ;;
    *)
      echo "Unknown argument: $arg"
      echo "Usage: scripts/start-docker.sh [--build] [--foreground]"
      exit 1
      ;;
  esac
done

if [[ ! -f .env && -f .env.example ]]; then
  cp .env.example .env
  echo "Created .env from .env.example. Fill OPENAI_API_KEY/JWT_SECRET before using AI features."
fi

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
  local container
  container="$(docker_container_using_port "$port" || true)"
  if [[ -n "$container" ]]; then
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

release_port() {
  local port="$1"
  local name="$2"
  stop_docker_container_using_port "$port"
  stop_local_port_owner "$port" "$name"
}

stop_compose_service_if_running() {
  local service="$1"
  if docker compose ps -q "$service" >/dev/null 2>&1 && [[ -n "$(docker compose ps -q "$service" 2>/dev/null)" ]]; then
    echo "Stopping existing Docker service '$service'..."
    run_checked docker compose stop "$service"
  fi
}

run_checked docker version >/dev/null
stop_compose_service_if_running worker
release_port 5432 PostgreSQL
release_port 6380 Redis
release_port 8081 API
release_port 3000 Frontend

args=(compose up)
if [[ "$BUILD" -eq 1 ]]; then
  args+=(--build)
fi
if [[ "$DETACHED" -eq 1 ]]; then
  args+=(-d)
fi

run_checked docker "${args[@]}"

echo
echo "NotebookMind Docker stack:"
echo "  Frontend: http://localhost:3000"
echo "  API:      http://localhost:8081/api/v1"
echo
echo "Logs: docker compose logs -f"
echo "Stop: docker compose down"
