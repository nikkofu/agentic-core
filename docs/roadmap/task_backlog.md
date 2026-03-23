# Agentic-Core Task Backlog

> Last updated: 2026-03-19
>
> This backlog translates the current source-of-truth documents into an execution sequence for the repository:
>
> - `docs/README.md`
> - `docs/enterprise_roadmap_agentic-core.md`
> - `docs/superpowers/plans/2026-03-18-runtime-governance-reset.md`
> - `docs/superpowers/specs/2026-03-18-llm-execution-kernel-design.md`

## Positioning

`agentic-core` should currently be built as an **Agent Runtime + Governance Kernel** first, and only then expanded into a broader multi-agent platform.

That means this backlog prioritizes:

1. truth in documentation
2. a provable gold path
3. governance evidence and session control
4. operational hardening
5. deferred platform expansion

## Prioritization Rules

### `P0` Source of truth

Fix documentation drift and broken source-of-truth entry points before adding new platform claims.

### `P1` Governance kernel

Close the gap between “events are emitted” and “the system is auditable, replayable, and session-stable”.

### `P2` Control-plane hardening

Upgrade provider routing, context management, and tool execution from MVP/demo quality to reusable kernel components.

### `P3` Platform expansion

Only after `P0`-`P2` are stable should the project expand into RAG, richer adapters, advanced multi-agent collaboration, and enterprise controls.

## Current Status Snapshot

### Landed foundations

- [x] Queue and event bus are split into separate abstractions.
- [x] `/v1/chat/completions` strict request validation is wired into the HTTP handler.
- [x] Approval webhook signature, timestamp, and nonce verification exist.
- [x] Unified `StreamChunk` fanout and SSE bridging exist.
- [x] Task terminal states preserve `timeout`, `cancelled`, and `rejected`.
- [x] Unified logging foundation is present.
- [x] WeCom / Feishu / DingTalk gateway slices exist.

### Partial implementations

- [ ] Audit events are emitted, but not yet queryable/replayable as a first-class evidence system.
- [ ] Session routing exists, but not as a sticky, distributed session-to-runtime control plane.
- [ ] Model routing exists, but only as a thin alias map over a very small provider surface.
- [ ] Context compaction exists, but is not yet token-budget aware.
- [ ] Session history exists, but still behaves like a local persistence helper rather than a governed conversation substrate.
- [ ] Tool execution exists, but builtin tools are still MVP/demo quality.

### Deferred or not-yet-wired capabilities

- [ ] Milvus long-term memory retrieval in the main execution path
- [ ] WASM as a real production tool backend
- [ ] production OpenTelemetry / metrics wiring
- [ ] user-facing `@agent` collaboration semantics
- [ ] Slack / Web channel productization
- [ ] quota, billing, rerank, and tenant controls

## Priority Overview

| ID | Priority | Theme | Status | Why It Matters Now |
| --- | --- | --- | --- | --- |
| BG-001 | P0 | Documentation truth reset | Missing / partial | The repo still overstates current platform capability and contains broken doc links |
| BG-103 | P1 | Gold-path proof suite | Partial / mostly landed | The core path already has strong regression coverage, but it still needs one explicit proof bundle and gap-driven follow-up |
| BG-101 | P1 | Audit evidence chain | Partial | Governance without durable evidence is not yet enterprise-grade |
| BG-102 | P1 | Sticky session routing | Missing | Gateway is still an ingress shim, not a true session router |
| BG-104 | P1 | Model resolver hardening | Partial | Provider abstraction is too thin for real operations |
| BG-201 | P2 | Token-aware context governance | Partial | Current compaction is count-based, not context-budget-based |
| BG-202 | P2 | Builtin tool execution hardening | Partial | Tooling is still MVP-grade and not yet a governed execution substrate |
| BG-203 | P2 | Session history hardening | Partial | History exists, but not yet as a recoverable conversation layer |
| BG-301 | P3 | Long-term memory / RAG | Deferred | Valuable, but explicitly not the current kernel priority |
| BG-302 | P3 | Production telemetry | Deferred | Needed later for operations, but not before the kernel is provable |
| BG-303 | P3 | Advanced multi-agent collaboration | Deferred | Current runtime still needs stronger core invariants before team semantics |
| BG-304 | P3 | Additional channel adapters | Deferred | More channels would amplify current control-plane gaps |
| BG-305 | P3 | Enterprise controls | Deferred | Quota, billing, rerank, and tenant controls should sit on a stable kernel |

## `P0` Documentation Truth Reset

### BG-001: Align docs with the actual runtime phase

**Goal:** Make the repository honest about what is implemented, what is partial, and what is intentionally deferred.

**Primary files**

- `README.md`
- `docs/README.md`
- `docs/roadmap/task_backlog.md`
- `docs/architecture/process_scheduling.md`
- `docs/protocol/internal_bus.md`
- `docs/journal/README.md`

**Tasks**

- [x] Create `docs/roadmap/task_backlog.md` as the active execution backlog.
- [x] Keep `docs/README.md` pointed at the reset plan and current execution anchors.
- [ ] Add missing placeholder or real docs for:
  - `docs/architecture/process_scheduling.md`
  - `docs/protocol/internal_bus.md`
  - `docs/journal/README.md`
- [ ] Update `README.md` to distinguish:
  - current runtime kernel capabilities
  - partial capabilities
  - long-term roadmap claims
- [ ] Add a current-state matrix in `docs/README.md` with categories:
  - implemented
  - partial
  - deferred
- [ ] Remove or downgrade any wording that implies:
  - production RAG
  - complete `@agent` collaboration
  - full WASM tool execution
  - production OTEL observability

**Acceptance**

- [ ] Core documentation links resolve.
- [ ] Root docs no longer claim deferred capabilities as if they already exist.
- [ ] New contributors can identify the current build priority within 5 minutes.

## `P1` Governance Kernel

### BG-101: Make audit a durable evidence chain

**Goal:** Turn audit from “structured events are emitted” into “a task can be reconstructed and investigated later”.

**Primary files**

- `internal/process/audit.go`
- `internal/process/audit_redactor.go`
- `internal/llm/contracts.go`
- `cmd/orchestrator/main.go`
- `cmd/subagent/main.go`
- new audit persistence and query files under `internal/process/` or `internal/memory/`

**Tasks**

- [ ] Define a durable audit storage model keyed by `TaskID`, `TraceID`, and event sequence.
- [ ] Persist audit events instead of only publishing/logging them.
- [ ] Add read-path support to fetch the full audit trail for one task.
- [ ] Add a minimal replay/debug helper:
  - CLI or HTTP endpoint
  - task timeline reconstruction
  - final state correlation
- [ ] Ensure approval, chunk, final, timeout, cancelled, and error events are all persisted consistently.
- [ ] Extend redaction coverage to sensitive request, tool, and approval payloads.

**Acceptance**

- [ ] A single `TaskID` can be queried and reconstructed end-to-end.
- [ ] Audit data is useful for postmortem and support triage, not just console viewing.
- [ ] The replay path is covered by tests.

### BG-102: Upgrade the gateway from ingress shim to sticky session router

**Goal:** Route repeated requests from the same conversation into a stable runtime identity instead of always creating a fresh task shell.

**Primary files**

- `internal/gateway/router.go`
- `internal/gateway/types.go`
- `cmd/gateway/main.go`
- `cmd/orchestrator/main.go`
- `internal/memory/` session or route persistence helpers

**Tasks**

- [ ] Introduce a durable session binding model:
  - `session_id`
  - active `task_id` or runtime identity
  - channel metadata
  - TTL / last-seen timestamp
- [ ] Add concurrency-safe session claiming to avoid duplicate runtime attachment.
- [ ] Distinguish:
  - new session request
  - in-flight existing session request
  - expired session reattachment
- [ ] Add cleanup / expiry behavior for stale route bindings.
- [ ] Define late-result handling when a route has expired or been replaced.
- [ ] Add tests for concurrent requests sharing the same `session_id`.

**Acceptance**

- [ ] Same-session requests are deterministically routed.
- [ ] Route state survives beyond an in-memory map.
- [ ] Gateway behavior no longer depends on a single process-local router map.

### BG-103: Prove the gold path with integrated regression coverage

**Goal:** Consolidate the already-landed regression coverage into one explicit proof suite, then add only the missing scenarios that are still unsupported.

**Primary files**

- `cmd/orchestrator/main_test.go`
- `cmd/subagent/main_test.go`
- `internal/gateway/chat_completions_handler_test.go`
- `internal/gateway/sender_test.go`
- `internal/llm/runtime_loop_test.go`
- integration scripts under `scripts/`

**Tasks**

- [x] Land focused regression coverage for:
  - happy-path result persistence
  - replay-style audit regression
  - approval-gated write path
  - timeout and late decision behavior
  - SSE disconnect / aborted stream behavior
  - unified chunk / SSE event-model behavior
- [ ] Document the existing proof buckets in one place with named commands or package targets.
- [ ] Add one real-transport or scripted smoke slice that complements the fake-transport-heavy test set.
- [ ] Add only the remaining missing scenario coverage once a concrete gap is demonstrated.
- [ ] Keep the proof suite aligned with future audit persistence and session-routing changes.

**Acceptance**

- [ ] Gold-path coverage is explicit, named, and easy to rerun.
- [ ] Remaining additions are gap-driven instead of duplicating already-landed regressions.
- [ ] The suite demonstrates both component confidence and end-to-end confidence.

### BG-104: Harden the model resolver into an operational component

**Goal:** Evolve the resolver from a thin alias lookup into a stable provider control plane.

**Primary files**

- `internal/llm/resolver.go`
- `internal/llm/routes.go`
- `internal/llm/provider.go`
- `internal/llm/openai_provider.go`
- new provider implementations or adapters under `internal/llm/`

**Tasks**

- [ ] Support more than one real OpenAI-compatible provider path.
- [ ] Add route loading from configuration instead of hard-coded boot wiring only.
- [ ] Introduce timeout and retry policy at the provider boundary.
- [ ] Add API key rotation / multiple credentials per provider alias.
- [ ] Normalize provider errors into a tighter internal contract.
- [ ] Add resolver tests for:
  - missing alias
  - missing provider
  - multiple providers
  - key failover
  - timeout behavior

**Acceptance**

- [ ] The resolver is no longer effectively single-provider.
- [ ] Provider switching does not require code edits.
- [ ] Operational failure modes are testable and explicit.

## `P2` Control-Plane Hardening

### BG-201: Replace count-based compaction with token-aware context governance

**Goal:** Make context trimming reflect actual token budget instead of only message count.

**Primary files**

- `internal/session/context_compactor.go`
- `internal/llm/context_guard.go`
- `internal/session/history_store.go`
- relevant tests under `internal/session/` and `internal/llm/`

**Tasks**

- [ ] Add token estimation or pluggable token accounting.
- [ ] Trigger compact based on budget thresholds, not only `KeepRecent`.
- [ ] Preserve a bounded recent window plus compacted summary.
- [ ] Record compact operations into audit events.
- [ ] Add tests for:
  - no compact needed
  - compact triggered
  - summary injection
  - large-history stability

**Acceptance**

- [ ] Context management behaves predictably under long conversations.
- [ ] Compact behavior is visible and auditable.

### BG-202: Harden builtin tool execution while keeping WASM deferred

**Goal:** Upgrade builtin tools from demo behavior to real controlled execution, while preserving a clean interface for future WASM integration without pulling WASM into the active mainline yet.

**Primary files**

- `internal/skill/builtin.go`
- `internal/skill/executor.go`
- `internal/skill/skill.go`
- `internal/llm/runtime_loop.go`

**Tasks**

- [ ] Replace mock-only `http_get` behavior with a real controlled execution path.
- [ ] Define execution policy boundaries for outbound network and write operations.
- [ ] Add capability metadata for builtin tools and future backends.
- [ ] Ensure builtin execution produces stable audit and tool-result semantics.
- [ ] Keep the WASM interface compile-safe and contract-stable, but do not wire `WazeroExecutor` into the mainline in this phase.
- [ ] Add regression coverage for:
  - builtin read tool
  - builtin write tool requiring approval

**Acceptance**

- [ ] Tool execution is no longer a demo-only facade.
- [ ] Mainline execution quality improves without violating the reset plan’s deferred WASM stance.

### BG-203: Harden session history into a recoverable conversation substrate

**Goal:** Make session history reliable enough to support long-lived conversations and debugging.

**Primary files**

- `internal/session/history_store.go`
- `cmd/subagent/main.go`
- `internal/llm/context_guard.go`

**Tasks**

- [ ] Define retention and trimming behavior for `session_history`.
- [ ] Persist richer metadata where needed:
  - trace id
  - tool messages
  - summary insertion markers
- [ ] Ensure history recovery is robust across process restarts.
- [ ] Add tests for ordering, recovery, and compaction interaction.

**Acceptance**

- [ ] Session history is stable across multi-turn and restart scenarios.
- [ ] History supports context reconstruction instead of only append-and-read.

## `P3` Platform Expansion

### BG-301: Wire long-term memory / RAG into the mainline

- [ ] Add embedding generation pipeline.
- [ ] Define what gets stored into Milvus and when.
- [ ] Add retrieval injection rules for runtime prompts.
- [ ] Add cost / latency guardrails before enabling by default.

### BG-302: Add production-grade telemetry and metrics

- [ ] Replace stdout-only tracing with collector/exporter-based wiring.
- [ ] Add service/resource attributes and environment config.
- [ ] Add latency, error, and queue metrics.
- [ ] Add Prometheus-friendly metrics exposure if chosen.

### BG-303: Build user-facing multi-agent collaboration semantics

- [ ] Define how `@agent` is parsed and authorized.
- [ ] Turn parent/child task structures into real collaboration flows.
- [ ] Add task graph inspection and lifecycle visibility.
- [ ] Prevent uncontrolled delegation loops.

### BG-304: Expand channel adapters beyond current scope

- [ ] Add Slack adapter.
- [ ] Add Web adapter.
- [ ] Standardize inbound auth, session ID derivation, and outbound send guarantees.

### BG-305: Add enterprise controls after the kernel is stable

- [ ] quota and budget controls
- [ ] tenant isolation and policy binding
- [ ] model usage accounting
- [ ] rerank and retrieval policy layers

## Recommended Delivery Sequence

### Phase A: Stop documentation drift

1. `BG-001` Documentation truth reset

### Phase B: Make the kernel explainable and controllable

2. `BG-103` Gold-path proof suite
3. `BG-101` Audit evidence chain
4. `BG-102` Sticky session routing
5. `BG-104` Model resolver hardening

### Phase C: Make the kernel resilient and reusable

6. `BG-201` Token-aware context governance
7. `BG-202` Builtin tool execution hardening
8. `BG-203` Session history hardening

### Phase D: Expand into platform capabilities

9. `BG-301` Long-term memory / RAG
10. `BG-302` Production telemetry
11. `BG-303` Advanced multi-agent collaboration
12. `BG-304` Additional adapters
13. `BG-305` Enterprise controls

## Not For The Current Mainline

These items should remain explicitly deferred until the governance kernel is stable:

- full RAG productization
- Slack/Web product rollout
- advanced team delegation UX
- quota/billing systems
- rerank optimization layers
- large-scale observability rollout

## Definition of Backlog Success

This backlog is considered healthy only if:

- [ ] it reflects the runtime-governance reset, not the old platform-first narrative
- [ ] it clearly distinguishes landed / partial / deferred work
- [ ] it prevents the team from jumping to RAG or platform expansion too early
- [ ] it can be used as the starting point for the next implementation plan without reinterpretation
