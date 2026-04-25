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

args=(compose up)
if [[ "$BUILD" -eq 1 ]]; then
  args+=(--build)
fi
if [[ "$DETACHED" -eq 1 ]]; then
  args+=(-d)
fi

docker "${args[@]}"

echo
echo "NotebookMind Docker stack:"
echo "  Frontend: http://localhost:3000"
echo "  API:      http://localhost:8081/api/v1"
echo
echo "Logs: docker compose logs -f"
echo "Stop: docker compose down"
