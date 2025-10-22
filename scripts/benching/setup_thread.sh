#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR=$(cd "$(dirname "$0")/../.." && pwd)
cd "$ROOT_DIR"

# defaults
DEFAULT_TARGET="http://localhost:8080"
DEFAULT_USER_ID="user1"
DEFAULT_BACKEND_API_KEY="sk_example"
DEFAULT_FRONTEND_API_KEY="pk_example"
DEFAULT_THREAD_ID="bench-thread"

# prompt for target url
read -r -p "Host endpoint [${DEFAULT_TARGET}]: " TARGET_URL
TARGET_URL=${TARGET_URL:-$DEFAULT_TARGET}

# prompt for backend api key
read -r -p "BACKEND API key [${DEFAULT_BACKEND_API_KEY}]: " BACKEND_API_KEY
BACKEND_API_KEY=${BACKEND_API_KEY:-$DEFAULT_BACKEND_API_KEY}

# prompt for frontend api key
read -r -p "FRONTEND API key [${DEFAULT_FRONTEND_API_KEY}]: " FRONTEND_API_KEY
FRONTEND_API_KEY=${FRONTEND_API_KEY:-$DEFAULT_FRONTEND_API_KEY}

# prompt for user id
read -r -p "USER_ID [${DEFAULT_USER_ID}]: " USER_ID
USER_ID=${USER_ID:-$DEFAULT_USER_ID}

# prompt for thread id
read -r -p "THREAD_ID [${DEFAULT_THREAD_ID}]: " THREAD_ID
THREAD_ID=${THREAD_ID:-$DEFAULT_THREAD_ID}

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

# create the thread
echo "Creating thread '$THREAD_ID' for user '$USER_ID'"
THREAD_RESPONSE=$(curl -s -X POST "${TARGET_URL%/}/v1/threads" \
  -H "Authorization: Bearer ${FRONTEND_API_KEY}" \
  -H "Content-Type: application/json" \
  -H "X-User-ID: ${USER_ID}" \
  -H "X-User-Signature: ${SIG_VAL}" \
  -d "{\"id\":\"${THREAD_ID}\",\"title\":\"Benchmark Thread\",\"author\":\"${USER_ID}\"}")

echo "Thread creation response: $THREAD_RESPONSE"

# check if thread was created successfully
if echo "$THREAD_RESPONSE" | grep -q '"id"'; then
  echo "✓ Thread '$THREAD_ID' created successfully"
  echo "You can now run the message benchmark with:"
  echo "  THREAD_ID=$THREAD_ID ./scripts/benching/run_local.sh"
else
  echo "✗ Failed to create thread"
  exit 1
fi