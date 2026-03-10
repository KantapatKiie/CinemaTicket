#!/usr/bin/env sh
set -eu

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"

cleanup() {
  docker compose -f "$ROOT_DIR/docker-compose.yml" stop mongo redis >/dev/null 2>&1 || true
  if [ -n "${BACKEND_PID:-}" ]; then
    kill "$BACKEND_PID" 2>/dev/null || true
  fi
  if [ -n "${FRONTEND_PID:-}" ]; then
    kill "$FRONTEND_PID" 2>/dev/null || true
  fi
}

trap cleanup INT TERM EXIT

docker compose -f "$ROOT_DIR/docker-compose.yml" up -d mongo redis >/dev/null

export PORT=8080
export MONGO_URI="mongodb://localhost:27017"
export MONGO_DB="cinema"
export REDIS_ADDR="localhost:6379"
export REDIS_PASSWORD=""
export REDIS_DB=0
export JWT_SECRET="dev-secret"
export FIREBASE_PROJECT_ID=""
export LOCK_TTL_SECONDS=300
export SEAT_ROWS=5
export SEAT_COLS=10

cd "$ROOT_DIR/backend"
go run ./cmd/server/main.go &
BACKEND_PID=$!

cd "$ROOT_DIR/frontend"
if [ ! -d node_modules ]; then
  npm install
fi
npm run dev -- --host 0.0.0.0 --port 5173 &
FRONTEND_PID=$!

wait "$BACKEND_PID" "$FRONTEND_PID"
