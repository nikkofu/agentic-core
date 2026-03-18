# LLM Execution Kernel Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在现有 `agentic-core` 中交付可运行的 LLM 执行内核：OpenAI-compatible 静态路由、ReAct 循环、SSE+内部 chunk 双通道、内置工具+HITL+WASM 三件套、以及可审计的确定性状态流。

**Architecture:** 保持 `cmd/subagent` 轻量 wiring，将内核下沉到 `internal/llm`（contracts/router/parser/runtime/stream）与 `internal/skill`（executor/approval）并通过 `internal/bus` 强类型消息贯通。按 spec 里程碑推进：先闭环、再审批安全、后 WASM、最后 compact+审计。全程 TDD（先失败测试、后最小实现、再回归）。

**Tech Stack:** Go 1.21+, context.Context, Redis Pub/Sub, SQLite (modernc), OpenAI-compatible HTTP API, SSE, wazero sandbox, Go testing.

---

## File Structure (planned changes)

### Create (or create-if-missing)
- `internal/llm/contracts.go` — InferenceRequest/ChatMessage/ToolCall/ToolResult/StreamChunk/AuditEvent
- `internal/llm/openai_compat.go` — chat/completions 严格字段验证 + 错误映射
- `internal/llm/routes.go` — 静态路由 alias 解析
- `internal/llm/schema_parser.go` — 模型输出解析（final/tool_call）
- `internal/llm/stream_fanout.go` — chunk 序列器 + dual fanout
- `internal/llm/runtime_loop.go` — ReAct 状态机
- `internal/skill/executor.go` — 统一执行器（先 builtin，后 wasm）
- `internal/skill/approval_gate.go` — 写操作审批 gate
- `internal/gateway/sse_handler.go` — SSE writer（event/data/[DONE]）
- `internal/gateway/sender.go` — 订阅内部 outbox 并对外发送
- `internal/session/context_compactor.go` — compact 策略
- `internal/process/audit_redactor.go` — 审计脱敏

### Modify (if absent, first create from existing pattern)
- `internal/llm/provider.go`
- `internal/llm/openai_provider.go`
- `internal/llm/resolver.go`
- `internal/llm/context_guard.go`
- `internal/skill/skill.go`
- `internal/skill/builtin.go`
- `internal/skill/wasm.go`
- `internal/bus/message.go`
- `internal/process/audit.go`
- `cmd/subagent/main.go`
- `cmd/subagent/main_test.go`
- `cmd/orchestrator/main.go`
- `cmd/orchestrator/main_test.go`
- `internal/bus/message_test.go`

### Tests to create
- `internal/llm/openai_compat_test.go`
- `internal/llm/routes_test.go`
- `internal/llm/schema_parser_test.go`
- `internal/llm/stream_fanout_test.go`
- `internal/llm/runtime_loop_test.go`
- `internal/skill/executor_test.go`
- `internal/skill/approval_gate_test.go`
- `internal/gateway/sse_handler_test.go`
- `internal/gateway/sender_test.go`
- `internal/session/context_compactor_test.go`
- `internal/process/audit_redactor_test.go`

---

### Task 0: Preflight path verification and scaffold sync

**Files:**
- Modify: `docs/superpowers/plans/2026-03-18-llm-execution-kernel.md` (checkbox update only during execution)

- [ ] **Step 1: Verify planned file paths exist or are creatable**

Run: `go test ./cmd/subagent ./cmd/orchestrator ./internal/bus -run TestParse -v`
Expected: baseline tests run; repository path assumptions are valid.

- [ ] **Step 2: If any planned "Modify" target is absent, create file with minimal package/type scaffold**

Use nearest existing package style; no feature logic yet.

- [ ] **Step 3: Run package compile smoke check**

Run: `go test ./internal/llm ./internal/skill ./internal/gateway ./internal/session ./internal/process -run '^$'`
Expected: compile success (or only expected missing-symbol errors in next task tests).

- [ ] **Step 4: Commit scaffold normalization**

```bash
git add internal/llm internal/skill internal/gateway internal/session internal/process
git commit -m "chore: normalize llm-kernel scaffold paths"
```

---

### Task 1: Lock OpenAI-compatible contracts and strict request validation

**Files:**
- Create: `internal/llm/contracts.go`
- Create: `internal/llm/openai_compat.go`
- Create: `internal/llm/openai_compat_test.go`
- Modify: `internal/llm/provider.go`

- [x] **Step 1: Write failing tests for strict request contract**

```go
func TestValidateChatCompletionRequestRejectsUnknownField(t *testing.T) {}
func TestValidateChatCompletionRequestRejectsInvalidToolChoice(t *testing.T) {}
func TestMapProviderErrorToHTTPStatus(t *testing.T) {}
```

- [x] **Step 2: Run tests and verify failures**

Run: `go test ./internal/llm -run 'TestValidateChatCompletionRequest|TestMapProviderErrorToHTTPStatus' -v`
Expected: FAIL with missing validator/mapper.

- [x] **Step 3: Implement minimal validator and mapper**

```go
func ValidateChatCompletionRequest(raw []byte) (ChatCompletionRequest, error)
func MapProviderError(err error) (status int, typ string, msg string)
```

- [x] **Step 4: Re-run tests**

Run: `go test ./internal/llm -run 'TestValidateChatCompletionRequest|TestMapProviderErrorToHTTPStatus' -v`
Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add internal/llm/contracts.go internal/llm/openai_compat.go internal/llm/openai_compat_test.go internal/llm/provider.go
git commit -m "feat(llm): add openai-compatible request validation and error mapping"
```

---

### Task 2: Add static model routing by alias

**Files:**
- Create: `internal/llm/routes.go`
- Create: `internal/llm/routes_test.go`
- Modify: `internal/llm/resolver.go`

- [x] **Step 1: Write failing routing tests**

```go
func TestResolveByAliasReturnsStaticRoute(t *testing.T) {}
func TestResolveByAliasReturnsErrorWhenMissing(t *testing.T) {}
```

- [x] **Step 2: Run failing tests**

Run: `go test ./internal/llm -run 'TestResolveByAlias' -v`
Expected: FAIL.

- [x] **Step 3: Implement route table + resolver binding**

```go
func (r *ModelResolver) RegisterRoute(route StaticRoute)
func (r *ModelResolver) ResolveByAlias(alias string) (Provider, StaticRoute, error)
```

- [x] **Step 4: Re-run tests**

Run: `go test ./internal/llm -run 'TestResolveByAlias' -v`
Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add internal/llm/routes.go internal/llm/routes_test.go internal/llm/resolver.go
git commit -m "feat(llm): support static model alias routing"
```

---

### Task 3: Implement schema parser for final/tool_call outputs

**Files:**
- Create: `internal/llm/schema_parser.go`
- Create: `internal/llm/schema_parser_test.go`
- Modify: `internal/llm/contracts.go`

- [x] **Step 1: Write failing parser tests**

```go
func TestParseModelOutputFinalOnly(t *testing.T) {}
func TestParseModelOutputToolCall(t *testing.T) {}
func TestParseModelOutputRejectsInvalidJSON(t *testing.T) {}
```

- [x] **Step 2: Run failing tests**

Run: `go test ./internal/llm -run 'TestParseModelOutput' -v`
Expected: FAIL.

- [x] **Step 3: Implement strict parser**

```go
func ParseModelOutput(raw string) (ActionEnvelope, error)
```

- [x] **Step 4: Re-run tests**

Run: `go test ./internal/llm -run 'TestParseModelOutput' -v`
Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add internal/llm/schema_parser.go internal/llm/schema_parser_test.go internal/llm/contracts.go
git commit -m "feat(llm): add deterministic model output parser"
```

---

### Task 4: Build stream fanout + SSE + internal sender

**Files:**
- Create: `internal/llm/stream_fanout.go`
- Create: `internal/llm/stream_fanout_test.go`
- Create: `internal/gateway/sse_handler.go`
- Create: `internal/gateway/sse_handler_test.go`
- Create: `internal/gateway/sender.go`
- Create: `internal/gateway/sender_test.go`
- Modify: `internal/bus/message.go`
- Modify: `internal/bus/message_test.go`

- [ ] **Step 1: Write failing stream contract tests**

```go
func TestChunkSequenceMonotonic(t *testing.T) {}
func TestTerminalBusinessEventUnique(t *testing.T) {}
func TestSSEWriterEmitsDoneFrame(t *testing.T) {}
func TestSenderForwardsOutboxChunk(t *testing.T) {}
```

- [ ] **Step 2: Run failing tests**

Run: `go test ./internal/llm ./internal/gateway ./internal/bus -run 'TestChunk|TestTerminal|TestSSE|TestSender' -v`
Expected: FAIL.

- [ ] **Step 3: Implement sequence generator + fanout + sender**

```go
func (f *Fanout) EmitDelta(...)
func (f *Fanout) EmitFinal(...)
func WriteSSEFrame(w io.Writer, event string, data []byte) error
func (s *Sender) Start(ctx context.Context) error
```

- [ ] **Step 4: Re-run tests**

Run: `go test ./internal/llm ./internal/gateway ./internal/bus -run 'TestChunk|TestTerminal|TestSSE|TestSender' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/llm/stream_fanout.go internal/llm/stream_fanout_test.go internal/gateway/sse_handler.go internal/gateway/sse_handler_test.go internal/gateway/sender.go internal/gateway/sender_test.go internal/bus/message.go internal/bus/message_test.go
git commit -m "feat(stream): add dual-channel chunk fanout, sender, and SSE framing"
```

---

### Task 4B: Deliver `/v1/chat/completions` endpoint (non-stream + stream)

**Files:**
- Create: `internal/gateway/chat_completions_handler.go`
- Create: `internal/gateway/chat_completions_handler_test.go`
- Modify: `cmd/orchestrator/main.go`
- Modify: `cmd/orchestrator/main_test.go`

- [ ] **Step 1: Write failing endpoint contract tests**

```go
func TestChatCompletionsNonStreamReturnsChoicesMessage(t *testing.T) {}
func TestChatCompletionsStreamEmitsSSEFramesAndDone(t *testing.T) {}
func TestChatCompletionsRejectsUnknownFields(t *testing.T) {}
```

- [ ] **Step 2: Run failing endpoint tests**

Run: `go test ./internal/gateway ./cmd/orchestrator -run 'TestChatCompletions' -v`
Expected: FAIL.

- [ ] **Step 3: Implement handler and orchestrator route wiring**

```go
func (h *ChatCompletionsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request)
```

Requirements:
- 路由：`POST /v1/chat/completions`
- 非流式：返回 OpenAI-compatible `choices[].message`
- 流式：SSE 帧输出 + 终止 `event: done` / `data: [DONE]`
- 错误：使用 `MapProviderError` 输出协议化错误

- [ ] **Step 4: Re-run endpoint tests**

Run: `go test ./internal/gateway ./cmd/orchestrator -run 'TestChatCompletions' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/gateway/chat_completions_handler.go internal/gateway/chat_completions_handler_test.go cmd/orchestrator/main.go cmd/orchestrator/main_test.go
git commit -m "feat(gateway): add openai-compatible chat completions endpoint"
```

---

### Task 5: Implement builtin-only unified tool executor (no WASM yet)

**Files:**
- Create: `internal/skill/executor.go`
- Create: `internal/skill/executor_test.go`
- Modify: `internal/skill/skill.go`
- Modify: `internal/skill/builtin.go`

- [ ] **Step 1: Write failing builtin executor tests**

```go
func TestExecutorRunsBuiltinTool(t *testing.T) {}
func TestExecutorReturnsErrorForUnknownTool(t *testing.T) {}
```

- [ ] **Step 2: Run failing tests**

Run: `go test ./internal/skill -run 'TestExecutor' -v`
Expected: FAIL.

- [ ] **Step 3: Implement builtin executor path**

```go
func (e *Executor) Execute(ctx context.Context, call llm.ToolCall) (llm.ToolResult, error)
```

- [ ] **Step 4: Re-run tests**

Run: `go test ./internal/skill -run 'TestExecutor' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/skill/executor.go internal/skill/executor_test.go internal/skill/skill.go internal/skill/builtin.go
git commit -m "feat(skill): add builtin unified tool executor"
```

---

### Task 6: Implement webhook security (HMAC + timestamp window + nonce replay guard)

**Files:**
- Create: `internal/skill/webhook_auth.go`
- Create: `internal/skill/webhook_auth_test.go`
- Modify: `cmd/orchestrator/main.go`
- Modify: `cmd/orchestrator/main_test.go`

- [ ] **Step 1: Write failing webhook security tests**

```go
func TestWebhookRejectsInvalidHMAC(t *testing.T) {}
func TestWebhookRejectsExpiredTimestamp(t *testing.T) {}
func TestWebhookRejectsReplayedNonce(t *testing.T) {}
func TestWebhookAcceptsValidSignedRequest(t *testing.T) {}
```

- [ ] **Step 2: Run failing tests**

Run: `go test ./cmd/orchestrator ./internal/skill -run 'TestWebhook' -v`
Expected: FAIL.

- [ ] **Step 3: Implement auth middleware and nonce store**

```go
func VerifyWebhookSignature(headers http.Header, body []byte, secret string, now time.Time) error
```

- [ ] **Step 4: Re-run tests**

Run: `go test ./cmd/orchestrator ./internal/skill -run 'TestWebhook' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/skill/webhook_auth.go internal/skill/webhook_auth_test.go cmd/orchestrator/main.go cmd/orchestrator/main_test.go
git commit -m "feat(hitl): add signed webhook auth with replay protection"
```

---

### Task 7: Add HITL approval gate with deterministic timeout/idempotency

**Files:**
- Create: `internal/skill/approval_gate.go`
- Create: `internal/skill/approval_gate_test.go`
- Modify: `internal/bus/message.go`
- Modify: `cmd/orchestrator/main.go`
- Modify: `cmd/orchestrator/main_test.go`

- [ ] **Step 1: Write failing approval-gate tests**

```go
func TestApprovalGateApprovesOnce(t *testing.T) {}
func TestApprovalGateIgnoresDuplicateDecision(t *testing.T) {}
func TestApprovalGateTimeoutReturnsTimeoutState(t *testing.T) {}
func TestApprovalGateLateDecisionIgnoredAfterTimeout(t *testing.T) {}
func TestApprovalGateConcurrentApproveRejectFirstWriterWins(t *testing.T) {}
```

- [ ] **Step 2: Run failing tests**

Run: `go test ./internal/skill ./cmd/orchestrator -run 'TestApprovalGate|TestApproval' -v`
Expected: FAIL.

- [ ] **Step 3: Implement approval wait/decision flow**

```go
func (g *ApprovalGate) WaitDecision(ctx context.Context, req ApprovalRequest) (ApprovalDecision, error)
```

Implementation constraints:
- 使用 `(TaskID, ToolCallID)` 作为持久化决策键
- 冲突决策（approve/reject 并发）采用 CAS 先写入成功者生效
- 失败写入路径必须审计 `ignored_conflict_decision`

- [ ] **Step 4: Re-run tests**

Run: `go test ./internal/skill ./cmd/orchestrator -run 'TestApprovalGate|TestApproval' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/skill/approval_gate.go internal/skill/approval_gate_test.go internal/bus/message.go cmd/orchestrator/main.go cmd/orchestrator/main_test.go
git commit -m "feat(hitl): add deterministic approval gate with timeout and idempotency"
```

---

### Task 8: Add WASM execution path to unified executor

**Files:**
- Modify: `internal/skill/executor.go`
- Modify: `internal/skill/executor_test.go`
- Modify: `internal/skill/wasm.go`

- [ ] **Step 1: Write failing WASM executor test**

```go
func TestExecutorRunsWasmTool(t *testing.T) {}
```

- [ ] **Step 2: Run failing tests**

Run: `go test ./internal/skill -run 'TestExecutorRunsWasmTool' -v`
Expected: FAIL.

- [ ] **Step 3: Implement wasm path in executor**

Ensure result shape is `llm.ToolResult` and errors are mapped consistently.

- [ ] **Step 4: Re-run tests**

Run: `go test ./internal/skill -run 'TestExecutor' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/skill/executor.go internal/skill/executor_test.go internal/skill/wasm.go
git commit -m "feat(skill): add wasm execution path to unified executor"
```

---

### Task 9: Implement deterministic ReAct runtime loop and subagent wiring

**Files:**
- Create: `internal/llm/runtime_loop.go`
- Create: `internal/llm/runtime_loop_test.go`
- Modify: `cmd/subagent/main.go`
- Modify: `cmd/subagent/main_test.go`

- [ ] **Step 1: Write failing state-machine tests**

```go
func TestRuntimeLoopFinalDirectly(t *testing.T) {}
func TestRuntimeLoopToolThenFinal(t *testing.T) {}
func TestRuntimeLoopApprovalRejectUsesPolicy(t *testing.T) {}
func TestRuntimeLoopApprovalTimeoutEndsInTimeout(t *testing.T) {}
func TestRuntimeLoopSSEFailureTransitionsToAbortedStream(t *testing.T) {}
func TestRuntimeLoopWriteToolNeverExecutesBeforeApproval_Builtin(t *testing.T) {}
func TestRuntimeLoopWriteToolNeverExecutesBeforeApproval_WASM(t *testing.T) {}
func TestRuntimeLoopStopsAtMaxTurns(t *testing.T) {}
```

- [ ] **Step 2: Run failing tests**

Run: `go test ./internal/llm ./cmd/subagent -run 'TestRuntimeLoop|TestSubagentRun' -v`
Expected: FAIL.

- [ ] **Step 3: Implement runtime loop + wiring**

```go
func (r *Runtime) Run(ctx context.Context, req InferenceRequest) (FinalResult, error)
```

- [ ] **Step 4: Re-run tests**

Run: `go test ./internal/llm ./cmd/subagent -run 'TestRuntimeLoop|TestSubagentRun' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/llm/runtime_loop.go internal/llm/runtime_loop_test.go cmd/subagent/main.go cmd/subagent/main_test.go
git commit -m "feat(runtime): add deterministic react loop and subagent wiring"
```

---

### Task 10: Add context compactor + guard integration

**Files:**
- Create: `internal/session/context_compactor.go`
- Create: `internal/session/context_compactor_test.go`
- Modify: `internal/llm/context_guard.go`
- Modify: `internal/session/history_store.go`

- [ ] **Step 1: Write failing compact tests**

```go
func TestCompactorKeepsRecentTurnsAndAddsSummary(t *testing.T) {}
func TestCompactorNoOpWhenUnderThreshold(t *testing.T) {}
```

- [ ] **Step 2: Run failing tests**

Run: `go test ./internal/session ./internal/llm -run 'TestCompactor|TestContextGuard' -v`
Expected: FAIL.

- [ ] **Step 3: Implement compactor and integration**

```go
func CompactHistory(messages []ChatMessage, maxTokens int, keepRecent int) (compacted []ChatMessage, usedSummary bool)
```

- [ ] **Step 4: Re-run tests**

Run: `go test ./internal/session ./internal/llm -run 'TestCompactor|TestContextGuard' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/session/context_compactor.go internal/session/context_compactor_test.go internal/llm/context_guard.go internal/session/history_store.go
git commit -m "feat(context): add compactor policy for long conversations"
```

---

### Task 11: Harden audit logging and run full regression in small buckets

**Files:**
- Create: `internal/process/audit_redactor.go`
- Create: `internal/process/audit_redactor_test.go`
- Modify: `internal/process/audit.go`
- Modify: `cmd/subagent/main_test.go`
- Modify: `cmd/orchestrator/main_test.go`
- Modify: `internal/bus/message_test.go`

- [ ] **Step 1: Write failing audit tests**

```go
func TestRedactorMasksAPIKeysAndSecrets(t *testing.T) {}
func TestLogActionIncludesStageAndStatus(t *testing.T) {}
```

- [ ] **Step 2: Run failing audit tests**

Run: `go test ./internal/process -run 'TestRedactor|TestLogAction' -v`
Expected: FAIL.

- [ ] **Step 3: Implement redaction and structured audit fields**

Run: `go test ./internal/process -run 'TestRedactor|TestLogAction' -v`
Expected: PASS.

- [ ] **Step 4: Add failing E2E tests for stream failure semantics**

```go
func TestE2E_SSEDisconnect_InternalChunkContinues(t *testing.T) {}
func TestE2E_AbortedStreamWithFinalEndsDone(t *testing.T) {}
func TestE2E_ApprovalLateDecisionIgnored(t *testing.T) {}
func TestE2E_WriteToolCannotBypassApproval_Builtin(t *testing.T) {}
func TestE2E_WriteToolCannotBypassApproval_WASM(t *testing.T) {}
```

- [ ] **Step 5: Fix E2E bucket A (approval idempotency/late decision)**

Run: `go test ./cmd/orchestrator ./internal/skill -run 'TestE2E_ApprovalLateDecisionIgnored|TestApproval' -v`
Expected: PASS.

- [ ] **Step 6: Fix E2E bucket B (stream terminal uniqueness/aborted stream)**

Run: `go test ./internal/llm ./internal/gateway -run 'TestE2E_SSEDisconnect_InternalChunkContinues|TestE2E_AbortedStreamWithFinalEndsDone|TestTerminal' -v`
Expected: PASS.

- [ ] **Step 7: Fix E2E bucket C (protocol + bus envelope compatibility)**

Run: `go test ./internal/llm ./internal/bus -run 'TestValidateChatCompletionRequest|TestMapProviderErrorToHTTPStatus|TestParseMessage' -v`
Expected: PASS.

- [ ] **Step 8: Run full test suite**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 9: Commit integration batch**

```bash
git add internal/process/audit_redactor.go internal/process/audit_redactor_test.go internal/process/audit.go cmd/subagent/main_test.go cmd/orchestrator/main_test.go internal/bus/message_test.go
git commit -m "test: add e2e coverage for streaming, hitl determinism, and audit safety"
```

---

## Execution Notes

- 实施必须使用 `@superpowers:subagent-driven-development`。
- 每个 Task 完成后立即运行对应最小测试再 commit。
- 写操作工具必须经过 approval gate。
- 审批 webhook 必须签名校验、时间窗口校验、nonce 防重放。
- 若实现行为与 spec 冲突，以 `docs/superpowers/specs/2026-03-18-llm-execution-kernel-design.md` 为准，先修测试再改代码。

## Plan-level DoD

- OpenAI-compatible 严格校验 + 错误映射可用
- 静态路由 alias->provider 可用
- ReAct 状态机（含 reject/timeout/cancel/aborted_stream）可预测
- SSE + 内部 chunk 双通道一致
- builtin + HITL + WASM 全链路可测
- compact 与审计脱敏通过
- `go test ./...` 全绿
