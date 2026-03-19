# Process Scheduling

## Current Runtime Model

The runtime-governance-reset plan (see `docs/superpowers/plans/2026-03-18-runtime-governance-reset.md`) frames the current phase as a single trusted agent runtime plus governance kernel. In that frame, the orchestrator is the HTTP/approval ingress, logging/audit publisher, and process supervisor, while each subagent binary handles an isolated LLM execution tied to a `TaskID`. The orchestrator owns the task queue (`tasks` / `task.<id>`) and result channel (`task_results`), and it only spawns a subagent process via `process.ExecProcessManager` when an incoming task needs work. The subagent is responsible for driving the LLM runtime, emitting `chunks.<TaskID>` events, and pushing completion status back onto `task_results`. This separation is the current process-boundary contract: the orchestrator never runs the runtime loop and the subagent never hosts HTTP endpoints.

## What Is Implemented Today

- The orchestrator parses CLI flags, wires Redis-backed `bus.NewRedisTransport`, and exposes `/v1/chat/completions` plus `/approval` handlers; approval decisions get published to the `approvals` topic (see `cmd/orchestrator/main.go`).
- Task execution occurs inside the subagent binary (`cmd/subagent/main.go`), which listens on `task.<TaskID>`, runs the runtime (`llm.Runtime`), records audit events, and publishes chunks to `chunks.<TaskID>` before enqueueing a `task_results` message.
- Process spawning is handled by `internal/process/exec_manager.go`, which calls the `subagent` binary via `exec.CommandContext` with `--agent-type` and `--task-id`. The orchestrator keeps track of these PIDs so cancellation or cleanup can target the right process without sharing runtime state.
- Workflow orchestration is currently scoped to `internal/workflow`, where `Workflow` builds DAG nodes, associates them with agent types, and drives the executor state machine once tasks are emitted. The orchestrator keeps a `map[string]*workflow.Workflow` and notifies nodes of success/failure, but each running node still corresponds to a single subagent process.

## Known Limits

- Task lifecycle control is still single-threaded inside the orchestrator; there is no distributed session routing or sticky runtime binding yet, so concurrent requests for the same `TaskID` race in the orchestrator’s in-memory map (see `docs/roadmap/task_backlog.md` for BG-102’s ambition).
- The runtime loop and approval gate run entirely inside the subagent, so any workflow-level decision (e.g., retrying a failed node, upgrading tooling, replaying a task) requires manual orchestration or additional tooling on top of the existing `Workflow` API.
- Cancel/timed-out agents rely on `cmd/subagent/main.go` closing once `context.Context` is cancelled; there is no supervisor that watches for runaway subagents beyond what `ExecProcessManager` tracks (deferred per Workstream 3 in the reset plan).
- Process scheduling decisions (e.g., when to scale subagents or queue backpressure) are not implemented yet—this phase is about documenting the hard boundary so later workstreams can focus on audit, routing, and proof without inventing a new scheduler now.

## Source-of-Truth References

- `docs/superpowers/plans/2026-03-18-runtime-governance-reset.md` documents the reset priorities and Workstream 0/1 scope that justify this current process split.
- `docs/roadmap/task_backlog.md` marks BG-002 and BG-102 as follow-on work for session routing and audit tracing around the orchestrator-subagent boundary.
- `docs/superpowers/specs/2026-03-18-llm-execution-kernel-design.md` defines the runtime and approval contracts that the subagent currently enacts.
