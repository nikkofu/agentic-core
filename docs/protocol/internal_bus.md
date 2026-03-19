# Internal Bus

## Current Bus Contract

Per the runtime-governance reset references (see `docs/superpowers/plans/2026-03-18-runtime-governance-reset.md` and `docs/roadmap/task_backlog.md`), the repository today enforces a clear split between a task queue and an event bus. The `internal/bus` package exposes two separate interfaces: `TaskQueue` for point-to-point work delivery, and `EventBus` for broadcast-style events. Redis-backed transports implement both, but only the queue semantics (`Enqueue`/`Dequeue`) are used for `tasks`, `task.<id>`, and `task_results`, while Pub/Sub-style `Publish`/`Subscribe` stays reserved for audit/approval/chunk topics.

### Queue Semantics

- `tasks` acts as the orchestrator’s central enqueue point for new TaskIDs (see `internal/process/exec_manager.go` and `cmd/orchestrator/main.go`). Incoming HTTP requests are translated into job payloads that are `Enqueue`d onto `tasks`, and the orchestrator waits for subagent results via `task_results`.
- `task.<id>` is a dedicated per-subagent channel. A subagent subscribes to `Dequeue(ctx, "task."+TaskID)` (see `cmd/subagent/main.go`) and runs the LLM runtime only when a message arrives, keeping the subagent execution tightly scoped to a single task.
- `task_results` is a shared queue that all subagents push their terminal outputs into via `Enqueue(ctx, "task_results", ...)`. The orchestrator consumes this queue to reconcile workflow nodes, audit state transitions, and emit final HTTP responses.

### Event Bus Semantics

- Broadcast events such as `approvals`, `chunks.<TaskID>`, and `system.health`/heartbeat channels remain on the event bus (`Publish`/`Subscribe`), honoring the split emphasized in the reset plan (Workstream 1). These topics use `bus.Message` envelopes with contributors like orchestrator, subagent, or approval gate acting as publishers.
- `approvals` is the shared topic where the orchestrator emits approval request payloads (in `cmd/orchestrator/main.go`) and where `internal/skill.NewApprovalGate` listens for decisions (see `internal/skill/approval_gate.go`). Approval decisions may be emitted back onto `approvals` for clients that replay or audit them.
- `chunks.<id>` topics stream runtime progress events emitted inside the subagent (`cmd/subagent/main.go`, `internal/gateway/sender.go`). The gateway’s sender subscribes to `chunks.*` and fanouts these events toward SSE clients while audit logging records each chunk event.

### Naming Expectations

- Queue topics: always use `tasks`, `task.<id>`, and `task_results` for task delivery and final consumption. No other module should treat `task_results` as an event stream.
- Event topics: use descriptive names such as `approvals`, `chunks.<TaskID>`, and `system.health`. Patterns may use wildcards only when subscribing (e.g., `chunks.*`).
- `tasks` and `task.<id>` messages embed `TaskID` and lightweight metadata, while `bus.TaskResult` enforces statuses like `success`, `failed`, `timeout`, `cancelled`, or `rejected`. The orchestrator normalizes those results using `memory.NormalizeTaskStatus` before reconciling workflow nodes.

## Observability & Audit Anchors

- Audit and approval events tie back to `docs/superpowers/specs/2026-03-18-llm-execution-kernel-design.md`, which describes the runtime and approval contracts that the current bus topics are supporting.
- The backlog document (`docs/roadmap/task_backlog.md`) keeps the split visible as a current-state truth, while the reset plan articulates why the split matters for `BG-001` and the governance-focused delivery sequence.
