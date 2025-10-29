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
        
        if (( j % 2 == 1 )); then
            AUTHOR="human"
            DETAILED_CONTENT="Can you explain why people procrastinate even when they know something is important?"
            ROLE="human"
        else
            AUTHOR="ai"
            DETAILED_CONTENT="Procrastination isn\u2019t just about being lazy or undisciplined \u2014 it\u2019s a complex self-regulation problem deeply tied to how our brains handle emotion, motivation, and time perception. Even when we rationally know something is important \u2014 finishing a report, replying to an email, filing taxes \u2014 we can still find ourselves avoiding it. That\u2019s because the decision to act isn\u2019t made by logic alone; it\u2019s made through the interaction between our limbic system (emotion-driven) and prefrontal cortex (logic-driven).\n\nLet\u2019s break it down:\n\n\uD83D\uDCE6 1. Emotional Avoidance at the Core\n\nAt its root, procrastination is a short-term emotional management strategy. When you see a task that feels unpleasant, uncertain, or effortful, your brain perceives it as a kind of threat \u2014 not in the physical sense, but emotionally. The limbic system then signals discomfort, which the brain tries to reduce by avoiding the task. So instead of starting, you scroll social media, clean the kitchen, or \u201cresearch\u201d productivity tools \u2014 anything that gives a quick dopamine reward and temporarily soothes that discomfort.\n\nYou\u2019re not being irrational \u2014 you\u2019re emotionally self-medicating.\n\n\uD83D\uDD70\uFE0F 2. The Time Illusion (Temporal Discounting)\n\nHumans are wired to value immediate rewards over future rewards \u2014 a phenomenon called temporal discounting. Even if finishing a project will make you feel great tomorrow, that emotional payoff feels distant. But the immediate reward of watching a video, checking your phone, or doing something easier feels real now.\n\nThis short-term bias is evolutionary \u2014 our ancestors prioritized immediate survival over long-term goals. Modern life, with its abstract deadlines and digital distractions, hijacks that mechanism constantly.\n\n\u2696\uFE0F 3. Perfectionism and Fear of Failure\n\nMany chronic procrastinators are not lazy \u2014 they\u2019re perfectionists. They delay starting because the task feels loaded with expectation: What if I can\u2019t do it perfectly? What if I mess up and it reflects badly on me? By delaying, they preserve an illusion of potential \u2014 \u201cI could have done it well if I had more time.\u201d It\u2019s a subtle form of self-protection. Starting removes the safety of possibility and exposes us to judgment or disappointment.\n\n\uD83E\uDDE0 4. Executive Dysfunction\n\nSometimes, procrastination stems from the brain\u2019s executive function system \u2014 the part that organizes, prioritizes, and initiates action. When you have ADHD, depression, anxiety, or even chronic stress, that system gets overloaded. You want to do the task, but your brain can\u2019t generate the mental activation energy to start. It\u2019s like pressing the gas pedal, and the engine revs \u2014 but the car doesn\u2019t move. That mismatch creates guilt, which increases stress, which further inhibits initiation \u2014 a feedback loop.\n\n\uD83D\uDCA1 5. The Paradox of Motivation\n\nWe often wait for motivation before acting, but neuroscience shows that action precedes motivation. Once you begin a task \u2014 even a small step \u2014 the brain starts producing dopamine related to progress. That sense of movement fuels motivation, not the other way around. This is why techniques like the \u201cjust five minutes\u201d rule or Pomodoro work \u2014 they lower the psychological entry cost and let your brain catch up emotionally after you start.\n\n\uD83D\uDD04 6. The Self-Criticism Loop\n\nWhen we procrastinate, we often add guilt and shame to the mix. We say things like \u201cI\u2019m so lazy\u201d or \u201cI always mess this up,\u201d which makes the task feel even heavier emotionally. That creates a double avoidance: not only are we avoiding the task, but we\u2019re also avoiding the feelings we\u2019ve created around the task.\n\nSelf-compassion, surprisingly, breaks this loop more effectively than self-discipline. When you forgive yourself for procrastinating, the emotional load lightens \u2014 and that makes starting again easier.\n\n\u2699\uFE0F 7. Practical Fixes That Actually Work\n\nHere are some approaches grounded in behavioral science:\n\nReduce task size: Break the task down into the smallest concrete step possible. Instead of \u201cwrite report,\u201d say \u201copen document\u201d or \u201cwrite first paragraph.\u201d The smaller the step, the less threat your brain perceives.\n\nTime-box it: Set a timer for 5\u201310 minutes and commit only to that window. The brain often continues after it starts \u2014 this is the \u201cZeigarnik effect,\u201d where incomplete tasks keep attention loops open.\n\nChange your environment: Procrastination thrives in distraction. A visual cue like a clean desk, full-screen app mode, or a dedicated workspace can help your brain associate the environment with focus.\n\nReward progress, not perfection: Celebrate small wins (e.g., writing 100 words) rather than only completed outcomes. Dopamine spikes for progress, not completion.\n\nExternalize accountability: Tell a friend, coworker, or even a digital tool that you\u2019ll update them in an hour. Social accountability activates a different motivational circuit.\n\nForgive yourself: Studies show that people who practice self-forgiveness after procrastination are less likely to repeat it in the future. Guilt adds friction; compassion adds energy.\n\n\uD83C\uDF05 8. Reframing Procrastination\n\nInstead of seeing procrastination as a flaw, think of it as a signal \u2014 something in the task feels unsafe, unclear, or unmanageable to your current emotional state. If you can identify what the avoidance is protecting you from (fear, boredom, uncertainty), you can design around it instead of fighting it head-on.\n\nProcrastination isn\u2019t the enemy. It\u2019s just your brain asking for emotional clarity before it can move forward.\n\nSo, in short:\n\nProcrastination is not a time-management problem \u2014 it\u2019s an emotion-management problem disguised as one. Once you treat the underlying emotion (not the symptom), the behavior starts to fix itself."
            ROLE="ai"
        fi
        MESSAGE_BODY=$(cat <<EOF
{
    "content": "",
    "body": {"role": "${ROLE}", "content": "${DETAILED_CONTENT}"},
    "author": "${AUTHOR}"
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