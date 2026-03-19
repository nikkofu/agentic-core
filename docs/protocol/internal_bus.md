# Internal Bus

## Current Bus Contract

Per the runtime-governance reset references (see `docs/superpowers/plans/2026-03-18-runtime-governance-reset.md` and `docs/roadmap/task_backlog.md`), the repository today enforces a clear split between a task queue and an event bus. The `internal/bus` package exposes two separate interfaces: `TaskQueue` for point-to-point work delivery, and `EventBus` for broadcast-style events. Redis-backed transports implement both, but only the queue semantics (`Enqueue`/`Dequeue`) are used for `tasks`, `task.<id>`, and `task_results`, while Pub/Sub-style `Publish`/`Subscribe` stays reserved for audit/approval/chunk topics.

### Queue Semantics

- `tasks` is the incoming queued work ingress for the routed adapter/session path—`internal/gateway/session_router.go`:100 enqueues channel messages here so the orchestrator can continue with workflow scheduling (see also `cmd/orchestrator/main.go:356`). No other handler currently writes to `tasks`; direct `/v1/chat/completions` flows that stay in the orchestrator skip this queue and run immediately inside the gateway runtime.
- `task.<id>` is a dedicated per-subagent channel. A subagent subscribes to `Dequeue(ctx, "task."+TaskID)` (see `cmd/subagent/main.go`) and runs the LLM runtime only when a workflow payload arrives, keeping the execution scope tied to that `TaskID`.
- `task_results` is a shared queue that subagents publish their terminal `bus.TaskResult` output onto via `Enqueue(ctx, "task_results", ...)`. The orchestrator’s `ListenResults` loop consumes those messages to update `memory.TaskStateStore`, drive `internal/workflow` node completion/failure, and emit audit events, while the gateway/session router also listens for the fanback on `task_results` to push replies back to external channels.

### Event Bus Semantics

- Broadcast events such as `chunks.<TaskID>` and `system.health`/heartbeat channels remain on the event bus (`Publish`/`Subscribe`), honoring the split emphasized in the reset plan (Workstream 1). These topics use `bus.Message` envelopes with contributors like orchestrator, subagent, or gateway sender acting as publishers.
- `approvals` is the shared topic that approval webhook handlers publish decisions onto (see `cmd/orchestrator/main.go`), and `internal/skill.ApprovalGate` subscribes to those decisions. Approval requests surface as `waiting_approval` stream chunks on `chunks.<TaskID>` rather than being queued onto `approvals`.
- `chunks.<id>` topics stream runtime progress events emitted inside the subagent (`cmd/subagent/main.go`) or gateway sender (`internal/gateway/sender.go`). The sender subscribes to `chunks.*`, fans those messages to SSE clients, and drives audit logging for each chunk event (`llm.StreamChunk`).
- Audit events publish either to `audit` (when no `TaskID` is associated) or `audit.<TaskID>` (when task-scoped), as implemented in `internal/process/audit.go`. Each `llm.AuditEvent` chooses exactly one topic before being logged so that listeners can subscribe to the desired scope without duplicate events.

### Naming Expectations

- Queue topics: always use `tasks`, `task.<id>`, and `task_results` for task delivery and final consumption. No other module should treat `task_results` as an event stream.
- Event topics: use descriptive names such as `approvals`, `chunks.<TaskID>`, and `system.health`. Patterns may use wildcards only when subscribing (e.g., `chunks.*`).
- `tasks` and `task.<id>` messages embed `TaskID` and lightweight metadata, while `bus.TaskResult` enforces statuses like `success`, `failed`, `timeout`, `cancelled`, or `rejected`. The orchestrator normalizes those results using `memory.NormalizeTaskStatus` before reconciling workflow nodes.

## Observability & Audit Anchors

- Audit and approval events tie back to `docs/superpowers/specs/2026-03-18-llm-execution-kernel-design.md`, which describes the runtime and approval contracts that the current bus topics are supporting.
- The backlog document (`docs/roadmap/task_backlog.md`) keeps the split visible as a current-state truth, while the reset plan articulates why the split matters for `BG-001` and the governance-focused delivery sequence.
