# Agentic-Core Phase A | Runtime Governance Reset

Phase A is about resetting the documentation truth: surface what the runtime/governance kernel already delivers, capture what is still being hardened, and clearly defer the rest of the platform ambitions until later phases.

## 当前阶段定位
The Runtime Governance Reset plan re-centers the project on verifiable capabilities rather than from-scratch grand narratives. Refer to the [Runtime Governance Reset plan](docs/superpowers/plans/2026-03-18-runtime-governance-reset.md), the [Task Backlog](docs/roadmap/task_backlog.md), and the [Phase A spec](docs/superpowers/specs/2026-03-19-phase-a-doc-truth-proof-design.md) to see how proof of those capabilities feeds into the gold_path_proof execution story. This document surfaces implemented, partial, and deferred tracks to keep contributors aligned.

## 已实现 / 可证明能力
- `POST /v1/chat/completions` remains the Phase A gold path for driving kernel-level requests, so this entry point is the factual locus of proofed traffic.
- Strict request validation on `POST /v1/chat/completions` enforces the schema, required fields, and gating expected by the backlog.
- Static model routing captures the current resolver surface while downstream hardening continues.
- Approval webhook signature verification, timeout, and late-decision handling are part of the proven flow between orchestrator and human approvers.
- Unified chunk/SSE event modeling powers the gateway slices and fanout that serve each `POST /v1/chat/completions` session.
- Task terminal states and unified logging foundation keep the runtime governance kernel traceable.
- Gateway channel slices (HTTP, WeCom, Feishu, DingTalk) deliver the same logged session narrative across channels.
- The logging foundation records every chunk, SSE event, and terminal transition needed for proofing.

## 部分实现能力
- Durable audit evidence chain is still being consolidated and awaits hardened replay tooling.
- Sticky session routing prototypes exist, but resilience and handshake proofs are in active hardening.
- Model resolver hardening continues to guarantee the correct backend selection per session.
- Token-aware context governance and builtin tool execution governance remain in-flight against the backlog’s safety contracts.
- Session history hardening remains in-flight so every conversation can reconstruct the input context unequivocally.

## 明确延后能力
- Milvus / RAG in the main execution path remains postponed until Phase B proof completion.
- WebAssembly (`wazero`) as the active production backend is deferred until governance safety fences are proven.
- Production OTEL observability and metrics pipelines are explicitly delayed; current logging remains the honest data source.
- Advanced multi-agent collaboration semantics (parallel streams, policy guards, `@agent` namespacing) are queued for later platform milestones.
- Additional channel productization and enterprise-grade controls beyond the kernel proof story are deferred until the runtime governance proof is ready for wider distribution.
