#!/usr/bin/env bash
# Watchtower-Lite local development entrypoint.
# Build/setup happens here; a Python supervisor owns the real OS processes.

set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
cd "$PROJECT_DIR"

RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m'

log() { echo -e "${BLUE}[watchtower]${NC} $*"; }
fail() { echo -e "${RED}[watchtower] $*${NC}" >&2; exit 1; }
require_command() { command -v "$1" >/dev/null 2>&1 || fail "缺少命令: $1"; }

require_command go
require_command node

mkdir -p .cache/go-build bin
if [[ -z "${GOCACHE:-}" ]]; then
    if command -v cygpath >/dev/null 2>&1; then
        export GOCACHE="$(cygpath -w "$PROJECT_DIR/.cache/go-build")"
    else
        export GOCACHE="$PROJECT_DIR/.cache/go-build"
    fi
fi

log "构建 Go 服务..."
go build -o bin/gateway ./cmd/gateway
go build -o bin/mock-infra ./cmd/mock-infra

log "准备 Python Brain..."
if [[ ! -d brain/.venv ]]; then
    require_command uv
    (cd brain && uv venv)
fi
if [[ -x brain/.venv/Scripts/python.exe ]]; then
    PYTHON_BIN="brain/.venv/Scripts/python.exe"
else
    PYTHON_BIN="brain/.venv/bin/python"
fi
if ! "$PYTHON_BIN" -c "import grpc, httpx" >/dev/null 2>&1; then
    if command -v uv >/dev/null 2>&1; then
        uv pip install --python "$PYTHON_BIN" -r requirements.txt
    else
        "$PYTHON_BIN" -m pip install -r requirements.txt
    fi
fi
"$PYTHON_BIN" -c "import proto.diagnosis_pb2" ||
    fail "protobuf 生成文件不可用，请执行 make proto"

log "读取 .env，并交给跨平台进程监督器检查端口、启动服务..."
exec "$PYTHON_BIN" scripts/supervisor.py
