# Agentic-Core Phase A | Runtime Governance Reset

Phase A is about resetting the documentation truth: surface what the runtime/governance kernel already delivers, capture what is still being hardened, and clearly defer the rest of the platform ambitions until later phases.

## 当前阶段定位
The Runtime Governance Reset plan re-centers the project on verifiable capabilities rather than from-scratch grand narratives. Refer to the [Runtime Governance Reset plan](docs/superpowers/plans/2026-03-18-runtime-governance-reset.md), the [Task Backlog](docs/roadmap/task_backlog.md), and the [Phase A spec](docs/superpowers/specs/2026-03-19-phase-a-doc-truth-proof-design.md) to see how proof of those capabilities feeds into the gold_path_proof execution story. This document surfaces implemented, partial, and deferred tracks to keep contributors aligned.

## 已实现 / 可证明能力
- Runtime/governance kernel forks and monitors sub-agents via `os/exec`, with `cmd/orchestrator` driving task dispatch and `cmd/subagent` providing the runtime entry point.
- `SQLite` stores the short-lived task state and metadata so the orchestrator can reflect on structured activity without speculative claims.
- `Redis Pub/Sub` and `MQTT` buses already circulate task assignments, heartbeats, and simple operational telemetry; the infrastructure is observable via existing text-based logs.
- Basic workflow orchestration, role-aware prompts, and manual `@` dispatch remain the current collaboration surface, with traceability kept in `internal/process` and `internal/workflow`.
- Proof stories live in `docs/testing/gold_path_proof.md`, ensuring every implemented block can be replayed or audited.

## 部分实现能力
- Collaboration semantics (`@agent` handoff and result delivery) are being hardened; the current shell is simple mention-based routing rather than a fully abstracted `@agent` contract.
- Long-memory / RAG capability now tracks prototype runs with Milvus, but the vector-store path is still under proof-of-concept review and not yet in the execution pipeline.
- WebAssembly (`wazero`) sandbox experiments run alongside `os/exec`, yet it is not the active execution backend for the orchestrator; hardening work continues.
- Observability beyond console logs (e.g., structured traces/metrics) is being prototyped with OpenTelemetry but is not production-ready; the focus remains on honest, human-readable telemetry while the OTEL layer stabilizes.

## 明确延后能力
- Production-grade RAG with Milvus or any vector database in the live execution path remains a deferred capability until Phase B.
- Full `@agent` collaboration semantics (parallel sub-agent streaming, policy guards, `@agent` namespacing) is queued for a later platform milestone.
- Treating WebAssembly as the active execution backend is postponed until governance and proof tooling can validate the runtime safety fences.
- Production OTEL observability/exporters for every runtime component is delayed; basic logging is the honest source of truth today.
- Enterprise-ready multi-agent platform marketing language (beyond the kernel + proof story) is deferred until the runtime/governance proof can be shared externally.
