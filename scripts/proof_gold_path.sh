#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$REPO_ROOT"

usage() {
  cat <<'EOF'
Usage: scripts/proof_gold_path.sh <bucket>

Buckets:
  approval-success
  approval-reject
  approval-timeout-late-decision
  audit-replay
  sse-abort
  gateway-route
  smoke
  all

Command outputs show readable headers for each bucket and stop on failure.
EOF
}

SMOKE_TEST_NAME="TestServeHTTPWriteToolApprovalWebhookSmokeOverLocalHTTP"

run_step() {
  local bucket="$1"
  shift

  echo "==> bucket: $bucket"
  for cmd in "$@"; do
    echo "--> $cmd"
    if ! bash -lc "$cmd"; then
      echo "FAIL: $bucket"
      exit 1
    fi
  done
  echo "PASS: $bucket"
}

run_required_pass_test() {
  local bucket="$1"
  local test_name="$2"
  local cmd="$3"
  local output

  echo "==> bucket: $bucket"
  echo "--> $cmd"
  if ! output=$(bash -lc "$cmd" 2>&1); then
    printf '%s\n' "$output"
    echo "FAIL: $bucket"
    exit 1
  fi
  printf '%s\n' "$output"
  if grep -Fq -- "--- SKIP: $test_name" <<<"$output"; then
    echo "FAIL: $bucket (required smoke test skipped)"
    exit 1
  fi
  if ! grep -Fq -- "--- PASS: $test_name" <<<"$output"; then
    echo "FAIL: $bucket (required smoke test did not report PASS)"
    exit 1
  fi
  echo "PASS: $bucket"
}

approval_success_bucket() {
  run_step approval-success \
    "go test ./internal/gateway -run 'Test(ChatCompletionsHandlerExecutesWriteToolAfterApproval|ChatCompletionsHandlerStreamsWriteToolAfterApproval)' -count=1" \
    "go test ./cmd/orchestrator -run 'TestServeHTTPCompletesWriteToolAfterApprovalWebhook' -count=1"
}

approval_reject_bucket() {
  run_step approval-reject \
    "go test ./internal/gateway -run 'TestChatCompletionsHandlerRejectsWriteToolApprovalAndPersistsRejectedState' -count=1"
}

approval_timeout_late_decision_bucket() {
  run_step approval-timeout-late-decision \
    "go test ./internal/gateway -run 'TestChatCompletionsHandlerTimeoutDuringApprovalPersistsTimeoutAndIgnoresLateDecision' -count=1" \
    "go test ./cmd/orchestrator -run 'TestServeHTTPLateApprovalWebhookDoesNotChangeTimedOutWriteToolState' -count=1"
}

audit_replay_bucket() {
  run_step audit-replay \
    "go test ./cmd/orchestrator -run 'TestSingleTaskReplayAuditPreservesTerminalResultStatus' -count=1"
}

sse_abort_bucket() {
  run_step sse-abort \
    "go test ./internal/gateway -run 'Test(SenderDisconnectDoesNotEmitDoneFrame|SenderPublishesAbortedStreamAuditOnDisconnect|SenderPublishesDoneStatusAuditWhenFinalChunkDisconnects)' -count=1"
}

gateway_route_bucket() {
  run_step gateway-route \
    "go test ./internal/gateway -run 'Test(HandleIncomingTracksTaskRouteAndEnqueuesUnifiedPayload|StartStreamListenerRoutesRichMessageToRegisteredAdapter|StartStreamListenerDirectSendUsesExplicitChannelMessage|StartStreamListenerDirectSendOverridesRouteBinding)' -count=1"
}

smoke_bucket() {
  run_required_pass_test smoke "$SMOKE_TEST_NAME" \
    "go test ./cmd/orchestrator -run '^${SMOKE_TEST_NAME}$' -count=1 -v"
}

run_all_buckets() {
  approval_success_bucket
  approval_reject_bucket
  approval_timeout_late_decision_bucket
  audit_replay_bucket
  sse_abort_bucket
  gateway_route_bucket
  smoke_bucket
}

if [ $# -ne 1 ]; then
  usage
  exit 1
fi

selected_bucket="$1"

case "$selected_bucket" in
  approval-success) approval_success_bucket ;; 
  approval-reject) approval_reject_bucket ;; 
  approval-timeout-late-decision) approval_timeout_late_decision_bucket ;; 
  audit-replay) audit_replay_bucket ;; 
  sse-abort) sse_abort_bucket ;; 
  gateway-route) gateway_route_bucket ;; 
  smoke) smoke_bucket ;; 
  all) run_all_buckets ;; 
  *)
    echo "Unknown bucket: $selected_bucket"
    usage
    exit 1
    ;;
esac
