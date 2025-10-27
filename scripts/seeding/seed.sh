#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR=$(cd "$(dirname "$0")/.." && pwd)
cd "$ROOT_DIR"

# defaults
DEFAULT_TARGET="http://localhost:8080"
DEFAULT_USER_ID="user1"
DEFAULT_BACKEND_API_KEY="sk_example"
DEFAULT_FRONTEND_API_KEY="pk_example"
DEFAULT_THREAD_COUNT=5
DEFAULT_MESSAGES_PER_THREAD=10

echo "=== ProgressDB Data Seeding Script ==="
echo "This script creates threads and messages for testing"
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

# prompt for user key
read -r -p "USER_ID [${DEFAULT_USER_ID}]: " USER_ID
USER_ID=${USER_ID:-$DEFAULT_USER_ID}
export USER_ID

# prompt for thread count
read -r -p "Number of threads to create [${DEFAULT_THREAD_COUNT}]: " THREAD_COUNT
THREAD_COUNT=${THREAD_COUNT:-$DEFAULT_THREAD_COUNT}
export THREAD_COUNT

# prompt for messages per thread
read -r -p "Messages per thread [${DEFAULT_MESSAGES_PER_THREAD}]: " MESSAGES_PER_THREAD
MESSAGES_PER_THREAD=${MESSAGES_PER_THREAD:-$DEFAULT_MESSAGES_PER_THREAD}
export MESSAGES_PER_THREAD

# validate inputs
if ! [[ "$THREAD_COUNT" =~ ^[0-9]+$ ]] || [ "$THREAD_COUNT" -lt 1 ]; then
    echo "Error: Thread count must be a positive integer"
    exit 1
fi

if ! [[ "$MESSAGES_PER_THREAD" =~ ^[0-9]+$ ]] || [ "$MESSAGES_PER_THREAD" -lt 1 ]; then
    echo "Error: Messages per thread must be a positive integer"
    exit 1
fi

# get user signature
echo "Requesting signature for user '$USER_ID'"
if ! command -v curl >/dev/null 2>&1; then
  echo "curl required but not found"; exit 1
fi

SIG_JSON=$(curl -s -X POST "${TARGET_URL%/}/v1/_sign" \
  -H "Authorization: Bearer ${BACKEND_API_KEY}" \
  -H "Content-Type: application/json" \
  -d "{\"userId\":\"${USER_ID}\"}") || SIG_JSON=''

SIG_VAL=$(echo "$SIG_JSON" | sed -n 's/.*"signature":"\([^"]*\)".*/\1/p') || SIG_VAL=''

if [[ -z "$SIG_VAL" ]]; then
  echo "Failed to obtain signature; response: $SIG_JSON"
  exit 1
fi

export GENERATED_USER_SIGNATURE="$SIG_VAL"

echo "Creating $THREAD_COUNT threads with $MESSAGES_PER_THREAD messages each..."
echo ""

# arrays to store created thread IDs
THREAD_IDS=()

# create threads
for ((i=1; i<=THREAD_COUNT; i++)); do
    echo "Creating thread $i/$THREAD_COUNT..."
    
    THREAD_TITLE="seed-thread-$(date +%s)-$i"
    THREAD_BODY=$(cat <<EOF
{
    "title": "${THREAD_TITLE}",
    "author": "${USER_ID}"
}
EOF
)
    
    THREAD_RESPONSE=$(curl -s -X POST "${TARGET_URL%/}/v1/threads" \
        -H "Authorization: Bearer ${FRONTEND_API_KEY}" \
        -H "X-User-ID: ${USER_ID}" \
        -H "X-User-Signature: ${GENERATED_USER_SIGNATURE}" \
        -H "Content-Type: application/json" \
        -d "$THREAD_BODY")
    
    # extract thread ID from response
    THREAD_ID=$(echo "$THREAD_RESPONSE" | sed -n 's/.*"key":"\([^"]*\)".*/\1/p')
    
    if [[ -z "$THREAD_ID" ]]; then
        echo "Error creating thread $i: $THREAD_RESPONSE"
        exit 1
    fi
    
    # No sleep, immediately GET the thread info
    THREAD_INFO=$(curl -s -X GET "${TARGET_URL%/}/v1/threads/${THREAD_ID}" \
        -H "Authorization: Bearer ${FRONTEND_API_KEY}" \
        -H "X-User-ID: ${USER_ID}" \
        -H "X-User-Signature: ${GENERATED_USER_SIGNATURE}")
    
    # If the GET request fails, the provisional ID might have been converted
    if echo "$THREAD_INFO" | grep -q "not found\|error"; then
        echo "  ⚠ Thread $i: provisional ID $THREAD_ID may have been converted"
        # For now, we'll still try with the provisional ID, but this might fail
    fi
    
    THREAD_IDS+=("$THREAD_ID")
    echo "  ✓ Created thread: $THREAD_ID"
done

echo ""
echo "Creating messages..."

# No post-thread creation sleep; create messages immediately

# create messages for each thread
for thread_index in "${!THREAD_IDS[@]}"; do
    THREAD_ID="${THREAD_IDS[$thread_index]}"
    echo "Thread $((thread_index + 1))/$THREAD_COUNT ($THREAD_ID):"
    
    for ((j=1; j<=MESSAGES_PER_THREAD; j++)); do
        echo "  Creating message $j/$MESSAGES_PER_THREAD..."
        
        MESSAGE_CONTENT="This is message $j in thread $((thread_index + 1)) created at $(date)"
        MESSAGE_BODY=$(cat <<EOF
{
    "content": "${MESSAGE_CONTENT}",
    "body": {},
    "author": "${USER_ID}"
}
EOF
)
        
        MESSAGE_RESPONSE=$(curl -s -X POST "${TARGET_URL%/}/v1/threads/${THREAD_ID}/messages" \
            -H "Authorization: Bearer ${FRONTEND_API_KEY}" \
            -H "X-User-ID: ${USER_ID}" \
            -H "X-User-Signature: ${GENERATED_USER_SIGNATURE}" \
            -H "Content-Type: application/json" \
            -d "$MESSAGE_BODY")
        
        # extract message ID from response
        MESSAGE_ID=$(echo "$MESSAGE_RESPONSE" | sed -n 's/.*"key":"\([^"]*\)".*/\1/p')
        
        if [[ -z "$MESSAGE_ID" ]]; then
            echo "    ✗ Error creating message $j: $MESSAGE_RESPONSE"
        else
            echo "    ✓ Created message: $MESSAGE_ID"
        fi
    done
done

echo ""
echo "=== Seeding Complete ==="
echo "Created $THREAD_COUNT threads:"
for thread_id in "${THREAD_IDS[@]}"; do
    echo "  - $thread_id"
done
echo ""
echo "Each thread contains $MESSAGES_PER_THREAD messages"
echo ""
echo "Total: $((THREAD_COUNT * MESSAGES_PER_THREAD)) messages created"