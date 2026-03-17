#!/bin/bash
set -e

echo "--- 🚀 Agentic-Core Team Collaboration Demo ---"

echo "1. Building binaries..."
go build -o orchestrator ./cmd/orchestrator
go build -o subagent ./cmd/subagent

echo "2. Starting Orchestrator (log to orch.log)..."
./orchestrator -loop > orch.log 2>&1 &
ORCH_PID=$!

# Ensure cleanup on exit
trap "kill $ORCH_PID; echo 'Stopped Orchestrator'" EXIT

echo "3. Dispatching Main Task to @orchestrator..."
TASK_ID="team_task_$(date +%s)"
# Payload mentions @researcher
PAYLOAD="{\"agent_type\":\"orchestrator\", \"task\":\"Prepare a report. First, @researcher please find Go 1.26 release notes.\"}"

# Push to main task queue
redis-cli LPUSH tasks "{\"message_id\":\"$TASK_ID\", \"sender_id\":\"user\", \"receiver_id\":\"orchestrator\", \"target_agent\":\"orchestrator\", \"payload\": $PAYLOAD, \"timestamp\": $(date +%s%3N)}"

echo "4. Pushing task content to agent channel..."
redis-cli LPUSH task.$TASK_ID "{\"message_id\":\"$TASK_ID\", \"sender_id\":\"orchestrator\", \"receiver_id\":\"$TASK_ID\", \"payload\": $PAYLOAD, \"timestamp\": $(date +%s%3N)}"

echo "5. Monitoring execution (10s)..."
sleep 10
echo "6. Final log output:"
cat orch.log | grep -E "Spawning Agent|spawned as PID|Subtask|finished with status"
kill $ORCH_PID || true
echo "Demo finished."
exit 0
