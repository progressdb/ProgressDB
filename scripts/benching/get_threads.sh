#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR=$(cd "$(dirname "$0")/../.." && pwd)
cd "$ROOT_DIR"

# defaults
DEFAULT_TARGET="http://localhost:8080"
DEFAULT_USER_ID="user1"
DEFAULT_BACKEND_API_KEY="sk_example"
DEFAULT_FRONTEND_API_KEY="pk_example"

echo "=== Get Threads Benchmark ==="
echo "This benchmark retrieves threads"
echo ""

# prompt for target url
read -r -p "Host endpoint [${DEFAULT_TARGET}]: " TARGET_URL
TARGET_URL=${TARGET_URL:-$DEFAULT_TARGET}
export TARGET_URL

# prompt for backend api key
read -r -p "BACKEND API key [${DEFAULT_BACKEND_API_KEY}]: " BACKEND_API_KEY
BACKEND_API_KEY=${BACKEND_API_KEY:-$DEFAULT_BACKEND_API_KEY}
export BACKEND_API_KEY

# prompt for frontend api key
read -r -p "FRONTEND API key [${DEFAULT_FRONTEND_API_KEY}]: " FRONTEND_API_KEY
FRONTEND_API_KEY=${FRONTEND_API_KEY:-$DEFAULT_FRONTEND_API_KEY}
export FRONTEND_API_KEY

# prompt for user id
read -r -p "USER_ID [${DEFAULT_USER_ID}]: " USER_ID
USER_ID=${USER_ID:-$DEFAULT_USER_ID}
export USER_ID

# get user signature
echo "Requesting signature for user '$USER_ID'"
if ! command -v curl >/dev/null 2>&1; then
  echo "curl required but not found"; exit 1
fi
SIG_JSON=$(curl -s -X POST "${TARGET_URL%/}/v1/_sign" -H "Authorization: Bearer ${BACKEND_API_KEY}" -H "Content-Type: application/json" -d "{\"userId\":\"${USER_ID}\"}") || SIG_JSON=''
SIG_VAL=$(echo "$SIG_JSON" | sed -n 's/.*"signature":"\([^"]*\)".*/\1/p') || SIG_VAL=''
if [[ -z "$SIG_VAL" ]]; then
  echo "Failed to obtain signature; response: $SIG_JSON"; exit 1
fi
export GENERATED_USER_SIGNATURE="$SIG_VAL"

# ensure artifacts root is available for outputs
ARTIFACT_ROOT=${TEST_ARTIFACTS_ROOT:-${PROGRESSDB_ARTIFACT_ROOT:-"$ROOT_DIR/tests/artifacts"}}
mkdir -p "$ARTIFACT_ROOT"
ARTIFACT_ROOT="$(cd "$ARTIFACT_ROOT" && pwd)"
export PROGRESSDB_ARTIFACT_ROOT="$ARTIFACT_ROOT"
export TEST_ARTIFACTS_ROOT="$ARTIFACT_ROOT"

# run k6 and save output beneath perf artifacts
ART_DIR="$ARTIFACT_ROOT/perf/k6"
mkdir -p "$ART_DIR"
TEST_ID="get-threads-$(date +%Y%m%d%H%M%S)"
OUT_JSON="$ART_DIR/${TEST_ID}.json"

echo "Running get-threads benchmark..."
k6 run --out json=$OUT_JSON service/tests/benching/k6/get-threads.js

echo "Output: $OUT_JSON"