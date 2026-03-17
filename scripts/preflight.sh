#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"

failures=0

pass() {
  printf 'PASS: %s\n' "$1"
}

fail() {
  printf 'FAIL: %s\n' "$1"
  failures=$((failures + 1))
}

check_dir() {
  rel="$1"
  if [ -d "$ROOT_DIR/$rel" ]; then
    pass "required path exists: $rel"
  else
    fail "missing required path: $rel"
  fi
}

check_file_contains() {
  file="$1"
  needle="$2"
  desc="$3"
  if [ ! -f "$file" ]; then
    fail "$desc (file not found: ${file#$ROOT_DIR/})"
    return
  fi

  if grep -Fq "$needle" "$file"; then
    pass "$desc"
  else
    fail "$desc"
  fi
}

printf 'Running bootstrap preflight checks in %s\n' "$ROOT_DIR"

# 1) Required paths
check_dir "cmd/orchestrator"
check_dir "cmd/subagent"
check_dir "internal/process"
check_dir "internal/bus"
check_dir "internal/memory"
check_dir "internal/workflow"
check_dir "pkg"

# 2) go.mod module name
check_file_contains "$ROOT_DIR/go.mod" "module agentic-core" "go.mod declares module agentic-core"

# 3) ProcessManager signatures
check_file_contains "$ROOT_DIR/internal/process/manager.go" "SpawnAgent(ctx context.Context, agentType string, taskID string, extraArgs ...string) (pid int, err error)" "manager.go contains SpawnAgent signature"
check_file_contains "$ROOT_DIR/internal/process/manager.go" "KillAgent(pid int) error" "manager.go contains KillAgent signature"

# 4) Message fields with json tags
check_file_contains "$ROOT_DIR/internal/bus/message.go" "MessageID" "message.go contains MessageID field"
check_file_contains "$ROOT_DIR/internal/bus/message.go" "SenderID" "message.go contains SenderID field"
check_file_contains "$ROOT_DIR/internal/bus/message.go" "ReceiverID" "message.go contains ReceiverID field"
check_file_contains "$ROOT_DIR/internal/bus/message.go" "Payload" "message.go contains Payload field"
check_file_contains "$ROOT_DIR/internal/bus/message.go" "Timestamp" "message.go contains Timestamp field"
check_file_contains "$ROOT_DIR/internal/bus/message.go" '`json:"message_id"`' "message.go includes json tag for MessageID"
check_file_contains "$ROOT_DIR/internal/bus/message.go" '`json:"sender_id"`' "message.go includes json tag for SenderID"
check_file_contains "$ROOT_DIR/internal/bus/message.go" '`json:"receiver_id"`' "message.go includes json tag for ReceiverID"
check_file_contains "$ROOT_DIR/internal/bus/message.go" '`json:"payload"`' "message.go includes json tag for Payload"
check_file_contains "$ROOT_DIR/internal/bus/message.go" '`json:"timestamp"`' "message.go includes json tag for Timestamp"

# 5) docker-compose service keys
check_file_contains "$ROOT_DIR/docker-compose.yml" "redis:" "docker-compose defines redis service key"
check_file_contains "$ROOT_DIR/docker-compose.yml" "mosquitto:" "docker-compose defines mosquitto service key"
check_file_contains "$ROOT_DIR/docker-compose.yml" "milvus:" "docker-compose defines milvus service key"

if [ "$failures" -gt 0 ]; then
  printf '\nPreflight FAILED with %d issue(s).\n' "$failures"
  exit 1
fi

printf '\nPreflight PASSED. All required bootstrap checks succeeded.\n'
