#!/usr/bin/env bash
# End-to-end local smoke test, including supervisor shutdown and port release.

set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
cd "$PROJECT_DIR"

LOG_FILE="$(mktemp)"
SUPERVISOR_PID=""

cleanup() {
    if [[ -n "$SUPERVISOR_PID" ]] && kill -0 "$SUPERVISOR_PID" 2>/dev/null; then
        kill -TERM "$SUPERVISOR_PID" 2>/dev/null || true
        wait "$SUPERVISOR_PID" 2>/dev/null || true
    fi
    rm -f "$LOG_FILE"
}
trap cleanup EXIT

./scripts/start.sh >"$LOG_FILE" 2>&1 &
SUPERVISOR_PID=$!

if ! node scripts/port-check.mjs wait 3000 8080 8081 9090 50052; then
    cat "$LOG_FILE"
    exit 1
fi

curl -fsS -X POST http://127.0.0.1:8080/api/v1/mock/trigger \
    -H 'Content-Type: application/json' \
    -d '{"scenario":"oom_kill","service":"payment"}' >/dev/null

sleep 3
INCIDENTS="$(curl -fsS http://127.0.0.1:8080/api/v1/incidents)"
if [[ "$INCIDENTS" != *'"service":"payment"'* ]] || [[ "$INCIDENTS" != *'"diagnosis"'* ]]; then
    echo "Smoke test did not observe the expected diagnosed incident."
    echo "$INCIDENTS"
    cat "$LOG_FILE"
    exit 1
fi

if ! kill -0 "$SUPERVISOR_PID" 2>/dev/null; then
    echo "Supervisor exited before shutdown verification."
    cat "$LOG_FILE"
    exit 1
fi
kill -TERM "$SUPERVISOR_PID"
wait "$SUPERVISOR_PID" || true
SUPERVISOR_PID=""

node scripts/port-check.mjs free 3000 8080 8081 9090 50052
echo "Smoke test passed: diagnosis completed and all ports were released."
