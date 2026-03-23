# Gold-Path Proof Guide

## What This Proves
This guide ties the Phase A runtime-governance proof buckets to the concrete tests and commands that anchor the proof in this worktree. Every bucket below is anchored to test files, line numbers, and names, and the commands listed in the Exact Commands section are the ones that `scripts/proof_gold_path.sh` reissues in this order.

## Proof Buckets

- **approval-success**
  - Tests:
    - `internal/gateway/chat_completions_handler_test.go:277` — `TestChatCompletionsHandlerExecutesWriteToolAfterApproval`
    - `internal/gateway/chat_completions_handler_test.go:442` — `TestChatCompletionsHandlerStreamsWriteToolAfterApproval`
    - `cmd/orchestrator/main_test.go:1022` — `TestServeHTTPCompletesWriteToolAfterApprovalWebhook`
  - Purpose: demonstrates that the gateway handler executes/streams the write tool and that the orchestrator HTTP stack completes the same write tool after receiving an approval webhook.
- **approval-reject**
  - Tests:
    - `internal/gateway/chat_completions_handler_test.go:564` — `TestChatCompletionsHandlerRejectsWriteToolApprovalAndPersistsRejectedState`
  - Purpose: proves rejected approvals persist the rejected state and never run the write tool.
- **approval-timeout-late-decision**
  - Tests:
    - `internal/gateway/chat_completions_handler_test.go:714` — `TestChatCompletionsHandlerTimeoutDuringApprovalPersistsTimeoutAndIgnoresLateDecision`
    - `cmd/orchestrator/main_test.go:1362` — `TestServeHTTPLateApprovalWebhookDoesNotChangeTimedOutWriteToolState`
  - Purpose: confirms gateway timeouts persist and ignore late decisions while the orchestrator HTTP stack preserves the timed-out write tool state against late approval notifications.
- **audit-replay**
  - Tests:
    - `cmd/orchestrator/main_test.go:706` — `TestSingleTaskReplayAuditPreservesTerminalResultStatus`
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
    - `cmd/orchestrator/main_test.go:1175` — `TestServeHTTPWriteToolApprovalWebhookSmokeOverLocalHTTP`
  - Purpose: ensures the orchestrator approval/write-tool success path crosses a distinct scripted local HTTP boundary over loopback, with the chat completion request and approval webhook both posted through a real HTTP server.
- **all**
  - Tests: the ordered composition of the bucket commands listed below.
  - Purpose: proves the entire gold path stays green when the buckets run sequentially.

## Exact Commands

Each command below is the exact call `scripts/proof_gold_path.sh` dispatches for its bucket. The `smoke` command requires loopback bind permission, and the runner counts a skipped smoke test as failure so the real HTTP boundary is never mistaken for a proved pass.

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
- `smoke`: `go test ./cmd/orchestrator -run '^TestServeHTTPWriteToolApprovalWebhookSmokeOverLocalHTTP$' -count=1 -v`
- `all`: `scripts/proof_gold_path.sh all` executes the above bucket commands in this order to compose the full gold-path proof.

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
  - Proves: the orchestrator HTTP path completes a write tool after an approval webhook across a real loopback HTTP server, distinct from the recorder-based approval success proof.
  - Does not prove: every webhook variant or SSE detail handled by the targeted buckets; if loopback binding is blocked, rerun outside the constrained sandbox rather than treating skip as proof.
- **all**
  - Proves: composing the bucket commands keeps the full gold path green.
  - Does not prove: end-to-end UI, load, or resilience scenarios beyond the scripted proof buckets.

## What Is Still Deferred

- `scripts/proof_gold_path.sh` is the landed runner for the documented buckets; keep this guide aligned with its commands whenever a bucket changes.
- End-to-end UI, load, and resilience scenarios remain out of scope for this Phase A proof; add new buckets and documentation if they become required.
- Any future bucket additions must be documented here before the script runner includes them so the gold-path spec stays aligned with the approved backlog.
