# Task Terminal State Alignment Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Unify task terminal state semantics across `gateway`, `subagent`, `orchestrator`, `task store`, and audit so `timeout`, `cancelled`, and `rejected` are preserved instead of being flattened into `failed`.

**Architecture:** Introduce one shared task-status contract in `internal/memory`, then make `cmd/subagent`, `cmd/orchestrator`, and `internal/gateway` reuse it instead of each inferring statuses independently. Keep workflow state coarse (`completed` vs `failed`) while preserving richer governance semantics in `TaskResult`, `TaskState`, and audit payloads. Execute this as small TDD slices: contract first, then subagent propagation, orchestrator persistence, gateway alignment, and final regression.

**Tech Stack:** Go 1.21+, standard `testing` package, existing `internal/llm`, `internal/memory`, `cmd/subagent`, `cmd/orchestrator`, `internal/gateway`, SQLite via `modernc`, Redis/MQTT fake transports in tests.

---

## File Map

- Create: `internal/memory/task_status.go` — shared task status constants/helpers/normalization.
- Create: `internal/memory/task_status_test.go` — focused contract tests for legal states and normalization.
- Modify: `internal/memory/task_state_store.go:11` — update `TaskState.Status` documentation to the full state set.
- Modify: `cmd/subagent/main.go:227` — preserve runtime-provided terminal statuses when emitting `task_results`.
- Modify: `cmd/subagent/main.go:313` — replace local status fallback logic with shared helpers.
- Modify: `cmd/subagent/main_test.go:228` — extend runtime terminal-state result coverage.
- Modify: `cmd/orchestrator/main.go:293` — preserve normalized terminal statuses when consuming `task_results`.
- Modify: `cmd/orchestrator/main.go:417` — remove flattening of `timeout/rejected/cancelled` into `failed`.
- Modify: `cmd/orchestrator/main_test.go` — update/add tests for preserved terminal states and unknown-status fallback.
- Modify: `internal/gateway/chat_completions_handler.go:579` — align gateway task-state inference with the shared contract.
- Modify: `internal/gateway/chat_completions_handler_test.go:563` — keep rejected/timeout regression coverage green under the shared helper.

### Task 0: Preflight and baseline verification

**Files:**
- Modify: `docs/superpowers/plans/2026-03-19-task-terminal-state-alignment.md` (checkbox updates only during execution)

- [ ] **Step 1: Verify the repo is clean before touching code**

Run: `git status --short --branch`
Expected: clean working tree on the execution branch/worktree.

- [ ] **Step 2: Run the current focused baseline**

Run: `go test ./internal/memory ./cmd/subagent ./cmd/orchestrator ./internal/gateway -run 'Test(SubagentRunPreservesRuntimeTimeoutStatusInTaskResult|ChatCompletionsHandlerRejectsWriteToolApprovalAndPersistsRejectedState|ChatCompletionsHandlerTimeoutDuringApprovalPersistsTimeoutAndIgnoresLateDecision)' -count=1`
Expected: PASS, confirming the starting point before changing shared status semantics.

- [ ] **Step 3: Record the execution scope**

Keep changes limited to shared task-status helpers plus the subagent/orchestrator/gateway call sites listed above.

### Task 1: Build the shared task status contract

**Files:**
- Create: `internal/memory/task_status.go`
- Create: `internal/memory/task_status_test.go`
- Modify: `internal/memory/task_state_store.go:11`

- [ ] **Step 1: Write failing tests for legal statuses and fallback behavior**

```go
func TestNormalizeTaskStatusPreservesKnownStates(t *testing.T) {}
func TestNormalizeTaskStatusFallsBackToFailed(t *testing.T) {}
func TestIsTerminalTaskStatus(t *testing.T) {}
func TestIsSuccessfulTaskStatus(t *testing.T) {}
```

- [ ] **Step 2: Run the contract tests and verify they fail**

Run: `go test ./internal/memory -run 'Test(NormalizeTaskStatus|IsTerminalTaskStatus|IsSuccessfulTaskStatus)' -count=1`
Expected: FAIL because the shared status contract does not exist yet.

- [ ] **Step 3: Implement the minimal shared status helpers**

```go
const (
    TaskStatusPending   = "pending"
    TaskStatusRunning   = "running"
    TaskStatusSuccess   = "success"
    TaskStatusFailed    = "failed"
    TaskStatusRejected  = "rejected"
    TaskStatusTimeout   = "timeout"
    TaskStatusCancelled = "cancelled"
)

func NormalizeTaskStatus(status string) string { ... }
func IsTerminalTaskStatus(status string) bool { ... }
func IsSuccessfulTaskStatus(status string) bool { ... }
```

- [ ] **Step 4: Update `TaskState.Status` documentation to the full state set**

```go
Status string // pending, running, success, failed, rejected, timeout, cancelled
```

- [ ] **Step 5: Re-run the contract tests**

Run: `go test ./internal/memory -run 'Test(NormalizeTaskStatus|IsTerminalTaskStatus|IsSuccessfulTaskStatus)' -count=1`
Expected: PASS.

- [ ] **Step 6: Commit the shared contract**

```bash
git add internal/memory/task_status.go internal/memory/task_status_test.go internal/memory/task_state_store.go
git commit -m "feat(memory): add shared task status contract"
```

### Task 2: Preserve runtime terminal states in subagent results

**Files:**
- Modify: `cmd/subagent/main.go:227`
- Modify: `cmd/subagent/main.go:313`
- Modify: `cmd/subagent/main_test.go:228`

- [ ] **Step 1: Add failing subagent tests for rejected and cancelled propagation**

```go
func TestSubagentRunPreservesRuntimeRejectedStatusInTaskResult(t *testing.T) {}
func TestSubagentRunPreservesRuntimeCancelledStatusInTaskResult(t *testing.T) {}
```

Reuse the existing timeout test pattern in `cmd/subagent/main_test.go:228`.

- [ ] **Step 2: Run the focused subagent tests and verify the new cases fail**

Run: `go test ./cmd/subagent -run 'TestSubagentRunPreservesRuntime(Timeout|Rejected|Cancelled)StatusInTaskResult' -count=1`
Expected: FAIL for the newly added cases until shared status preservation is wired in.

- [ ] **Step 3: Replace local fallback logic with the shared status contract**

```go
func runtimeTaskResultStatus(runtimeStatus string, err error) string {
    normalized := memory.NormalizeTaskStatus(runtimeStatus)
    if runtimeStatus != "" {
        return normalized
    }
    if err != nil {
        return memory.TaskStatusFailed
    }
    return memory.TaskStatusSuccess
}
```

- [ ] **Step 4: Ensure successful runs also emit the shared constant**

```go
s.sendResult(msg, memory.TaskStatusSuccess, result.Content, "")
```

- [ ] **Step 5: Re-run the focused subagent tests**

Run: `go test ./cmd/subagent -run 'TestSubagentRunPreservesRuntime(Timeout|Rejected|Cancelled)StatusInTaskResult' -count=1`
Expected: PASS.

- [ ] **Step 6: Commit the subagent propagation change**

```bash
git add cmd/subagent/main.go cmd/subagent/main_test.go
git commit -m "fix(subagent): preserve runtime terminal task states"
```

### Task 3: Preserve terminal states in orchestrator persistence and workflow mapping

**Files:**
- Modify: `cmd/orchestrator/main.go:293`
- Modify: `cmd/orchestrator/main.go:417`
- Modify: `cmd/orchestrator/main_test.go`

- [ ] **Step 1: Add failing orchestrator tests for preserved terminal states and unknown fallback**

```go
func TestListenResultsPreservesTimeoutTaskState(t *testing.T) {}
func TestListenResultsPreservesRejectedTaskState(t *testing.T) {}
func TestListenResultsPreservesCancelledTaskState(t *testing.T) {}
func TestListenResultsNormalizesUnknownTaskStateToFailed(t *testing.T) {}
```

These should follow the existing `ListenResults` test style in `cmd/orchestrator/main_test.go`, reusing fake queue + task store assertions.

- [ ] **Step 2: Run the focused orchestrator tests and verify failures**

Run: `go test ./cmd/orchestrator -run 'TestListenResults(Preserves|Normalizes)' -count=1`
Expected: FAIL because `normalizeTaskStatus` still collapses unknown and governance-specific terminal states.

- [ ] **Step 3: Replace local normalization with the shared contract**

```go
state := memory.TaskState{
    Status: memory.NormalizeTaskStatus(res.Status),
}
```

Remove the local `normalizeTaskStatus` helper from `cmd/orchestrator/main.go:417`.

- [ ] **Step 4: Keep workflow mapping coarse but explicit**

```go
if memory.IsSuccessfulTaskStatus(res.Status) {
    _ = wf.MarkCompleted(ctx, res.TaskID)
} else {
    _ = wf.MarkFailed(ctx, res.TaskID, res.Error)
}
```

Use the normalized status to decide completion vs failure, without extending workflow state enums.

- [ ] **Step 5: Re-run the focused orchestrator tests**

Run: `go test ./cmd/orchestrator -run 'TestListenResults(Preserves|Normalizes)' -count=1`
Expected: PASS.

- [ ] **Step 6: Run the broader orchestrator regression around gateway-backed terminal states**

Run: `go test ./cmd/orchestrator -run 'Test(ServeHTTPCompletesWriteToolAfterApprovalWebhook|ServeHTTPLateApprovalWebhookDoesNotChangeTimedOutWriteToolState)' -count=1`
Expected: PASS.

- [ ] **Step 7: Commit the orchestrator persistence change**

```bash
git add cmd/orchestrator/main.go cmd/orchestrator/main_test.go
git commit -m "fix(orchestrator): preserve terminal task states in store"
```

### Task 4: Align gateway task status inference with the shared contract

**Files:**
- Modify: `internal/gateway/chat_completions_handler.go:579`
- Modify: `internal/gateway/chat_completions_handler_test.go`

- [ ] **Step 1: Add focused gateway tests for shared status alignment**

```go
func TestTaskStatusForErrorUsesSharedContractForTimeout(t *testing.T) {}
func TestTaskStatusForErrorUsesSharedContractForRejected(t *testing.T) {}
func TestTaskStatusForErrorUsesSharedContractForCancelled(t *testing.T) {}
```

Keep the existing high-value regressions in `internal/gateway/chat_completions_handler_test.go:563` and `internal/gateway/chat_completions_handler_test.go:713`.

- [ ] **Step 2: Run the focused gateway tests and verify they fail or expose duplicate logic**

Run: `go test ./internal/gateway -run 'Test(TaskStatusForErrorUsesSharedContract|ChatCompletionsHandlerRejectsWriteToolApprovalAndPersistsRejectedState|ChatCompletionsHandlerTimeoutDuringApprovalPersistsTimeoutAndIgnoresLateDecision)' -count=1`
Expected: FAIL for the new helper-alignment tests until gateway uses the shared contract.

- [ ] **Step 3: Refactor gateway status helpers to reuse the shared contract**

```go
func taskStatusForResult(result llm.FinalResult, err error) string {
    if result.Status != "" {
        return memory.NormalizeTaskStatus(result.Status)
    }
    return taskStatusForError(err)
}
```

Use shared constants/normalization in `taskStatusForError` instead of hard-coded string returns where possible.

- [ ] **Step 4: Re-run the focused gateway tests**

Run: `go test ./internal/gateway -run 'Test(TaskStatusForErrorUsesSharedContract|ChatCompletionsHandlerRejectsWriteToolApprovalAndPersistsRejectedState|ChatCompletionsHandlerTimeoutDuringApprovalPersistsTimeoutAndIgnoresLateDecision)' -count=1`
Expected: PASS.

- [ ] **Step 5: Commit the gateway alignment**

```bash
git add internal/gateway/chat_completions_handler.go internal/gateway/chat_completions_handler_test.go
git commit -m "refactor(gateway): align task status inference with shared contract"
```

### Task 5: Run integrated verification and handoff

**Files:**
- Test: `internal/memory/*.go`
- Test: `cmd/subagent/*.go`
- Test: `cmd/orchestrator/*.go`
- Test: `internal/gateway/*.go`

- [ ] **Step 1: Run focused package verification**

Run: `go test ./internal/memory ./cmd/subagent ./cmd/orchestrator ./internal/gateway -count=1`
Expected: PASS.

- [ ] **Step 2: Run the full regression suite**

Run: `go test ./... -timeout 30s`
Expected: PASS.

- [ ] **Step 3: Review git diff for scope control**

Run: `git diff --stat origin/main...HEAD`
Expected: changes are limited to the shared status contract and the planned call sites/tests.

- [ ] **Step 4: Final handoff note**

Document in the execution summary that:

- `workflow` remains coarse (`completed` vs failed path)
- `TaskState` now preserves `rejected/timeout/cancelled`
- unknown legacy statuses still normalize safely to `failed`
