#!/usr/bin/env bash
set -euo pipefail

# Run a comparison of net/http vs fasthttp health endpoints using k6.
# Requires: go, k6, curl
# Defaults:
#  - NETHTTP_ADDR=:8082
#  - FASTHTTP_ADDR=:8081
#  - K6_SCRIPT=server/tests/benching/k6/health.js

NETHTTP_ADDR=${NETHTTP_ADDR:-":8082"}
FASTHTTP_ADDR=${FASTHTTP_ADDR:-":8081"}
K6_SCRIPT=${K6_SCRIPT:-server/tests/benching/k6/health.js}
BUILD_DIR=${BUILD_DIR:-/tmp}

echo "Building POC servers..."
go build -o "$BUILD_DIR/health-nethttp" ./server/cmd/health-nethttp
go build -o "$BUILD_DIR/health-fasthttp" ./server/cmd/health-fasthttp

NETHTTP_BIN="$BUILD_DIR/health-nethttp"
FASTHTTP_BIN="$BUILD_DIR/health-fasthttp"

NETHTTP_LOG=$(mktemp /tmp/nethttp.XXXX.log)
FASTHTTP_LOG=$(mktemp /tmp/fasthttp.XXXX.log)

echo "Starting net/http on $NETHTTP_ADDR (log: $NETHTTP_LOG)"
"$NETHTTP_BIN" -addr "$NETHTTP_ADDR" -version nethttp >"$NETHTTP_LOG" 2>&1 &
NETHTTP_PID=$!

echo "Starting fasthttp on $FASTHTTP_ADDR (log: $FASTHTTP_LOG)"
"$FASTHTTP_BIN" -addr "$FASTHTTP_ADDR" -version fasthttp >"$FASTHTTP_LOG" 2>&1 &
FASTHTTP_PID=$!

cleanup() {
  echo "Stopping servers..."
  kill $NETHTTP_PID 2>/dev/null || true
  kill $FASTHTTP_PID 2>/dev/null || true
}
trap cleanup EXIT

echo "Waiting for servers to become ready..."
for i in {1..30}; do
  if curl -sSf "http://127.0.0.1${NETHTTP_ADDR}/health" >/dev/null 2>&1 && curl -sSf "http://127.0.0.1${FASTHTTP_ADDR}/health" >/dev/null 2>&1; then
    echo "both healthy"
    break
  fi
  sleep 0.2
done

if ! command -v k6 >/dev/null 2>&1; then
  echo "k6 not found in PATH — please install k6 to run the benchmarks. Exiting." >&2
  exit 2
fi

OUT_DIR=${OUT_DIR:-bench_results}
mkdir -p "$OUT_DIR"

run_k6() {
  local target=$1
  local out=$2
  echo "Running k6 against $target -> $out"
  TARGET="$target" k6 run "$K6_SCRIPT" | tee "$out"
}

NETOUT="$OUT_DIR/nethttp_k6.out"
FASTOUT="$OUT_DIR/fasthttp_k6.out"

run_k6 "http://127.0.0.1${NETHTTP_ADDR}" "$NETOUT"
run_k6 "http://127.0.0.1${FASTHTTP_ADDR}" "$FASTOUT"

echo "Benchmarks complete. Logs:"
echo "  net/http log: $NETHTTP_LOG"
echo "  fasthttp log: $FASTHTTP_LOG"
echo "  net/http k6: $NETOUT"
echo "  fasthttp k6: $FASTOUT"

