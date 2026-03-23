# Agentic-Core 文档中心

欢迎来到 Agentic-Core 的核心规划与真相中心。本阶段工作聚焦于“Runtime Governance Reset”而非把未完成的功能当作既成事实。

## 当前执行锚点
- [Runtime Governance Reset Plan](./superpowers/plans/2026-03-18-runtime-governance-reset.md)：当前核心定位与治理重置的执行计划。
- [Task Backlog](./roadmap/task_backlog.md)：分解的任务清单与优先级，用于跟踪什么时候可以将“partial” 项目推向“implemented”。
- [Phase A Spec](./superpowers/specs/2026-03-19-phase-a-doc-truth-proof-design.md)：经过批准的 Phase A 规范，指导文档与 proof 的边界。
- [docs/testing/gold_path_proof.md](./testing/gold_path_proof.md)：当前的 Phase A gold-path proof guide，列出 proof buckets、对应测试与 exact commands，是验证 runtime governance proof 的主入口。
- [scripts/proof_gold_path.sh](../scripts/proof_gold_path.sh)：可执行的 proof runner，按 guide 中定义的 bucket 顺序运行对应命令。

## 当前运行注记
- [docs/architecture/process_scheduling.md](./architecture/process_scheduling.md)：当前 runtime 的进程调度边界、orchestrator/subagent 分工与已知限制。
- [docs/protocol/internal_bus.md](./protocol/internal_bus.md)：当前 queue/event bus 语义、topic 命名与审批/审计/chunk 事件边界。
- [docs/journal/README.md](./journal/README.md)：当前 journal 目录的定位说明，以及后续历史记录的承接位置。

## 当前状态矩阵
状态矩阵把项目能力划为 implemented / partial / deferred，以便在文档中保持一致：

| 轨道 | 状态 | 说明 |
| --- | --- | --- |
| Kernel runtime + governance | implemented | `POST /v1/chat/completions` gold path, strict validation/approval callbacks, unified chunk/SSE fanout, terminal states, gateway channel slices, and the logging foundation anchor the traceable kernel proof. |
| Governance hardening | partial | Durable audit evidence chain, sticky session routing, and model resolver hardening remain active proof tasks. |
| Context / session / tool hardening | partial | Token-aware context governance, builtin tool governance, and session history hardening continue to be shaped against the backlog’s safety contracts. |
| Memory / RAG | deferred | Milvus/RAG in the main execution path is postponed until Phase B proof completion. |
| Sandbox backend | deferred | WebAssembly (`wazero`) serving as the production backend is deferred until governance safety fences are proven. |
| Observability | deferred | Production OTEL/metrics pipelines are delayed; current logging is the honest source of truth. |

## 证明入口
- [docs/testing/gold_path_proof.md](./testing/gold_path_proof.md)：该文档列出 proof buckets、目的说明及当前使用的 exact commands，直接作为 gold-path proof 的可信入口。
- [scripts/proof_gold_path.sh](../scripts/proof_gold_path.sh)：该脚本按文档中的 bucket 顺序执行 proof 命令，便于复跑和留痕。
- 所有进一步的 proof 资料和日志应回到上述计划、Task Backlog 与 Phase A Spec，确保每条声明都有人负责验证。
