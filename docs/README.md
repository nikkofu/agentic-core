# Agentic-Core 文档中心

欢迎来到 Agentic-Core 的核心规划与真相中心。本阶段工作聚焦于“Runtime Governance Reset”而非把未完成的功能当作既成事实。

## 当前执行锚点
- [Runtime Governance Reset Plan](./superpowers/plans/2026-03-18-runtime-governance-reset.md)：当前核心定位与治理重置的执行计划。
- [Task Backlog](./roadmap/task_backlog.md)：分解的任务清单与优先级，用于跟踪什么时候可以将“partial” 项目推向“implemented”。
- [Phase A Spec](./superpowers/specs/2026-03-19-phase-a-doc-truth-proof-design.md)：经过批准的 Phase A 规范，指导文档与 proof 的边界。
- [docs/testing/gold_path_proof.md](./testing/gold_path_proof.md)：gold_path_proof 是本阶段对照 Proof Story 的实际执行入口，直接链接跳转。

## 当前状态矩阵
状态矩阵把项目能力划为 implemented / partial / deferred，以便在文档中保持一致：

| 轨道 | 状态 | 说明 |
| --- | --- | --- |
| Kernel runtime + governance | implemented | 控制层通过 `os/exec` 子进程、Redis/MQTT 总线、SQLite 状态存储可证明地运转，体现真实的 runtime 能力。 |
| Collaboration、memory、sandbox | partial | `@agent` 语法仍是简单调度，Milvus 的向量搜素只是原型，Wasm 沙盒也还在实验中；OpenTelemetry 也是试验性集成。 |
| Deferred platform capabilities | deferred | 全面 production RAG/Milvus 执行链、完整的 `@agent` 协作语义、Wasm 作为 default backend、production OTEL observability 等已经被明确延后。 |

## 证明入口
- [docs/testing/gold_path_proof.md](./testing/gold_path_proof.md)：gold_path_proof 文档记录了如何重放当前 runtime 证明链，并对照 Runtime Governance Reset 与 Task Backlog 中的 implemented/partial/deferred 状态。
- 所有进一步的 proof 资料和日志应回到上述计划、Task Backlog 与 Phase A Spec，确保每条声明都有人负责验证。
