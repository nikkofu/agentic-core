# Gold-Path Proof Guide

## What This Proves
This guide ties the Phase A runtime-governance proof buckets to the existing tests and commands that were already run successfully in this worktree. Every bucket below is anchored to concrete test files, line numbers, and names, and the commands listed in the Exact Commands section are the ones that Task 4's script runner will reissue in this order.

## Proof Buckets

- **approval-success**
  - Tests:
    - `internal/gateway/chat_completions_handler_test.go:277` — `TestChatCompletionsHandlerExecutesWriteToolAfterApproval`
    - `internal/gateway/chat_completions_handler_test.go:442` — `TestChatCompletionsHandlerStreamsWriteToolAfterApproval`
    - `cmd/orchestrator/main_test.go:1014` — `TestServeHTTPCompletesWriteToolAfterApprovalWebhook`
  - Purpose: demonstrates that the gateway handler executes/streams the write tool and that the orchestrator HTTP stack completes the same write tool after receiving an approval webhook.
- **approval-reject**
  - Tests:
    - `internal/gateway/chat_completions_handler_test.go:564` — `TestChatCompletionsHandlerRejectsWriteToolApprovalAndPersistsRejectedState`
  - Purpose: proves rejected approvals persist the rejected state and never run the write tool.
- **approval-timeout-late-decision**
  - Tests:
    - `internal/gateway/chat_completions_handler_test.go:714` — `TestChatCompletionsHandlerTimeoutDuringApprovalPersistsTimeoutAndIgnoresLateDecision`
    - `cmd/orchestrator/main_test.go:1167` — `TestServeHTTPLateApprovalWebhookDoesNotChangeTimedOutWriteToolState`
  - Purpose: confirms gateway timeouts persist and ignore late decisions while the orchestrator HTTP stack preserves the timed-out write tool state against late approval notifications.
- **audit-replay**
  - Tests:
    - `cmd/orchestrator/main_test.go:698` — `TestSingleTaskReplayAuditPreservesTerminalResultStatus`
  - Purpose: exercises the orchestrator audit replay path and confirms terminal result preservation.
- **sse-abort**
  - Tests:
    - `internal/gateway/sender_test.go:129` — `TestSenderDisconnectDoesNotEmitDoneFrame`
    - `internal/gateway/sender_test.go:175` — `TestSenderPublishesAbortedStreamAuditOnDisconnect`
    - `internal/gateway/sender_test.go:231` — `TestSenderPublishesDoneStatusAuditWhenFinalChunkDisconnects`
  - Purpose: covers sender behavior on disconnect, aborted-stream audits, and final chunk completion.
- **gateway-route**
  - Tests:
    - `internal/gateway/router_test.go:34` — `TestHandleIncomingTracksTaskRouteAndEnqueuesUnifiedPayload`
    - `internal/gateway/router_test.go:107` — `TestStartStreamListenerRoutesRichMessageToRegisteredAdapter`
    - `internal/gateway/router_test.go:206` — `TestStartStreamListenerDirectSendUsesExplicitChannelMessage`
    - `internal/gateway/router_test.go:274` — `TestStartStreamListenerDirectSendOverridesRouteBinding`
  - Purpose: validates router decisions, payload enqueueing, and both rich message and direct send routing.
- **smoke**
  - Tests:
    - `cmd/orchestrator/main_test.go:1014` — `TestServeHTTPCompletesWriteToolAfterApprovalWebhook`
  - Purpose: ensures the orchestrator HTTP stack completes the write tool after an approval webhook, crossing at least one real HTTP boundary rather than merely renaming an in-memory unit test. This intentionally reuses the orchestrator proof already listed under `approval-success` because `smoke` is the designated real-boundary slice.
- **all**
  - Tests: the ordered composition of the bucket commands listed below.
  - Purpose: proves the entire gold path stays green when the buckets run sequentially.

## Exact Commands

Each command below was verified manually in this worktree and is the exact call Task 4's runner will dispatch for its bucket.

- `approval-success`:
  - `go test ./internal/gateway -run 'Test(ChatCompletionsHandlerExecutesWriteToolAfterApproval|ChatCompletionsHandlerStreamsWriteToolAfterApproval)' -count=1`
  - `go test ./cmd/orchestrator -run 'TestServeHTTPCompletesWriteToolAfterApprovalWebhook' -count=1`
- `approval-reject`: `go test ./internal/gateway -run 'TestChatCompletionsHandlerRejectsWriteToolApprovalAndPersistsRejectedState' -count=1`
- `approval-timeout-late-decision`:
  - `go test ./internal/gateway -run 'TestChatCompletionsHandlerTimeoutDuringApprovalPersistsTimeoutAndIgnoresLateDecision' -count=1`
  - `go test ./cmd/orchestrator -run 'TestServeHTTPLateApprovalWebhookDoesNotChangeTimedOutWriteToolState' -count=1`
- `audit-replay`: `go test ./cmd/orchestrator -run 'TestSingleTaskReplayAuditPreservesTerminalResultStatus' -count=1`
- `sse-abort`: `go test ./internal/gateway -run 'Test(SenderDisconnectDoesNotEmitDoneFrame|SenderPublishesAbortedStreamAuditOnDisconnect|SenderPublishesDoneStatusAuditWhenFinalChunkDisconnects)' -count=1`
- `gateway-route`: `go test ./internal/gateway -run 'Test(HandleIncomingTracksTaskRouteAndEnqueuesUnifiedPayload|StartStreamListenerRoutesRichMessageToRegisteredAdapter|StartStreamListenerDirectSendUsesExplicitChannelMessage|StartStreamListenerDirectSendOverridesRouteBinding)' -count=1`
- `smoke`: `go test ./cmd/orchestrator -run 'TestServeHTTPCompletesWriteToolAfterApprovalWebhook' -count=1`
- `all`: script runner Task 4 will execute the above bucket commands in this order to compose the full gold-path proof.

## Coverage Boundaries

- **approval-success**
  - Proves: the chat completions handler runs the approved write tool and streams results as expected, and the orchestrator HTTP stack completes the same write tool after receiving the approval webhook.
  - Does not prove: every upstream delivery timing variant or orchestrator behavior beyond the webhook completion path.
- **approval-reject**
  - Proves: rejected approvals keep write tools inactive and persist rejection.
  - Does not prove: audit replay continuity or downstream router logic.
- **approval-timeout-late-decision**
  - Proves: timeout detection persists and ignores late webhook decisions, and the orchestrator preserves the timed-out write-tool state when late decisions arrive.
  - Does not prove: high-latency network behavior, which the `smoke` bucket covers by touching the real HTTP boundary.
- **audit-replay**
  - Proves: orchestrator audit replay preserves terminal task status.
  - Does not prove: gateway routing or SSE behaviors.
- **sse-abort**
  - Proves: sender disconnects neither emit done nor drop audits, with final chunk cleanups covered.
  - Does not prove: approvals in the orchestrator or router decisions.
- **gateway-route**
  - Proves: routing of new tasks, adapter notifications, and direct-send overrides behave as expected.
  - Does not prove: completion of write tools or audit state transitions beyond routing.
- **smoke**
  - Proves: the orchestrator HTTP path completes a write tool after an approval webhook across a real boundary; it cannot be satisfied by only renaming an in-memory unit test.
  - Does not prove: every webhook variant or SSE detail handled by the targeted buckets.
- **all**
  - Proves: composing the bucket commands keeps the full gold path green.
  - Does not prove: that the Task 4 runner itself has shipped (that work is deferred to the script implementation).

## What Is Still Deferred

- Task 4 needs to implement and run the script that dispatches the bucket commands in the order documented above.
- End-to-end UI, load, and resilience scenarios remain out of scope for this Phase A proof; add new buckets and documentation if they become required.
- Any future bucket additions must be documented here before the script runner includes them so the gold-path spec stays aligned with the approved backlog.
