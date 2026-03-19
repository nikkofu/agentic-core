# Process Scheduling

## Current Runtime Model

The runtime-governance-reset plan (see `docs/superpowers/plans/2026-03-18-runtime-governance-reset.md`) frames the current phase as a single trusted agent runtime plus governance kernel. In that frame the orchestrator is the HTTP/approval ingress, logging/audit publisher, and process supervisor, while queued workflows spawn dedicated subagent binaries to keep their `llm.Runtime` executions isolated. The orchestrator owns the task queue (`tasks` / `task.<id>`) and result channel (`task_results`) for that queued path, while the gateway/chat handler can also execute the runtime loop and approval gate directly when handling `/v1/chat/completions` requests that stay entirely inside the orchestrator process. The current boundary therefore balances two routes: queued workflow tasks that run in dedicated subagents, and direct HTTP requests that stay inside the orchestrator/gateway stack.

## What Is Implemented Today

- The orchestrator parses CLI flags, wires Redis-backed `bus.NewRedisTransport`, and exposes `/v1/chat/completions` plus `/approval` handlers (see `cmd/orchestrator/main.go`). Approval decisions flow through the `approvals` topic and the gateway’s `skill.ApprovalGate` participates directly when the runtime loop runs inside the orchestrator stack.
- Task execution via the queued workflow path is handled by the subagent binary (`cmd/subagent/main.go`), which listens on `task.<TaskID>`, runs the runtime (`llm.Runtime`), records audit events, publishes chunks to `chunks.<TaskID>`, and enqueues the terminal status back onto `task_results`.
- Process spawning is handled by `internal/process/exec_manager.go`, which calls the `subagent` binary via `exec.CommandContext` with `--agent-type` and `--task-id`. The orchestrator keeps track of these PIDs so cancellation or cleanup can target the right process without sharing runtime state.
- Workflow orchestration is currently scoped to `internal/workflow`, where `Workflow` builds DAG nodes, associates them with agent types, and drives the executor state machine once tasks are emitted for the queued path. The orchestrator keeps a `map[string]*workflow.Workflow` and notifies nodes of success/failure, while direct `/v1/chat/completions` requests may short-circuit this queue by staying inside the orchestrator/gateway runtime and emitting events via `llm.NewRuntime` + `skill.NewExecutor`.
- When the orchestrator handles runtime + approval directly (e.g., `handleNonStream`/`handleStream` in `internal/gateway/chat_completions_handler.go`), it runs `llm.NewRuntime` with `skill.NewExecutor` and `skill.NewApprovalGate`, writes audit/chunk events through `gateway.Sender`, and either streams SSE `chunks` or writes the final response without enqueuing onto `tasks`.

## Known Limits

- Task lifecycle control is still single-threaded inside the orchestrator; there is no distributed session routing or sticky runtime binding yet, so concurrent requests for the same `TaskID` race in the orchestrator’s in-memory map (see `docs/roadmap/task_backlog.md` for BG-102’s ambition).
- The runtime loop and approval gate execute inside whichever process currently owns the request: queued workflows delegate to subagents, while direct HTTP flows run them inside the orchestrator/gateway stack. Workflow-level retries or replays therefore still require manual orchestration across these boundaries.
- Cancel/timed-out agents rely on `cmd/subagent/main.go` closing once `context.Context` is cancelled; there is no supervisor that watches for runaway subagents beyond what `ExecProcessManager` tracks (deferred per Workstream 3 in the reset plan).
- Process scheduling decisions (e.g., when to scale subagents or queue backpressure) are not implemented yet—this phase is about documenting the hard boundary so later workstreams can focus on audit, routing, and proof without inventing a new scheduler now.

## Source-of-Truth References

- `docs/superpowers/plans/2026-03-18-runtime-governance-reset.md` documents the reset priorities and Workstream 0/1 scope that justify this current process split.
- `docs/roadmap/task_backlog.md` marks BG-102 as follow-on work for session routing and audit tracing around the orchestrator-subagent boundary.
- `docs/superpowers/specs/2026-03-18-llm-execution-kernel-design.md` defines the runtime and approval contracts that the subagent currently enacts.
