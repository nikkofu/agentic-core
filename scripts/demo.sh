#!/bin/bash
set -e

echo "1. Building binaries..."
go build -o orchestrator ./cmd/orchestrator
go build -o subagent ./cmd/subagent

echo "2. Starting Orchestrator in background (log to orch.log)..."
./orchestrator -loop > orch.log 2>&1 &
ORCH_PID=$!

# Ensure cleanup on exit
trap "kill $ORCH_PID; echo 'Stopped Orchestrator'" EXIT

echo "3. Sending a task via Redis..."
TASK_ID="task_$(date +%s)"
PAYLOAD="{\"agent_type\":\"researcher\", \"task\":\"Find latest Go 1.25 features\"}"

# 推送任务到主队列
redis-cli LPUSH tasks "{\"message_id\":\"$TASK_ID\", \"sender_id\":\"user\", \"receiver_id\":\"orchestrator\", \"payload\": $PAYLOAD, \"timestamp\": $(date +%s%3N)}"

echo "4. Pushing detailed task content to agent channel..."
# 子进程启动后会从这个频道获取具体任务内容
redis-cli LPUSH task.$TASK_ID "{\"message_id\":\"$TASK_ID\", \"sender_id\":\"orchestrator\", \"receiver_id\":\"$TASK_ID\", \"payload\": $PAYLOAD, \"timestamp\": $(date +%s%3N)}"

echo "5. Waiting for results (tailing orch.log)..."
timeout 15 tail -f orch.log | grep --line-buffered -E "Spawned|Received result"
