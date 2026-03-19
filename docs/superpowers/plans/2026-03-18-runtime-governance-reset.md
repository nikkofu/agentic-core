# Runtime Governance Reset Plan

> **Goal:** 将 `agentic-core` 从“概念先行的多智能体平台原型”收敛为“企业级 Agent 执行与治理内核”，优先打通一条可控、可终止、可审批、可审计、可回放的金路径。

## Why this reset

当前仓库最宝贵的资产，不是“多 Agent”这个词，也不是 Milvus / Wasm / Adapter 这些扩展能力，而是已经逐步形成的以下系统意识：

- 进程级隔离
- 强类型消息契约
- Human-in-the-loop 审批
- 审计与追责
- 上下文治理
- 明确的状态机与终态

但当前代码与文档仍存在明显偏差，导致系统更像“架构原型”而不是“可信内核”：

- Redis 队列语义与 Pub/Sub 语义混用
- 任务结果通道命名不一致
- 子进程取消路径不可靠
- 审批 webhook 契约与审批等待契约不一致
- OpenAI strict validation 没有真正进入 HTTP 入口
- 内部 chunk 事件与外部 SSE 事件尚未统一
- fake 实现与真实实现语义不一致

本计划的核心原则是：**先做硬约束，再做高能力。**

---

## Repositioning

### New project statement

`agentic-core` 的近期目标不再表述为“企业级多智能体平台”，而是：

**企业级 Agent 执行与治理内核（Agent Runtime + Governance Kernel）**

### What this means

项目的第一优先级不再是：

- 更多渠道接入
- 更多 Agent 角色
- 更多工具
- 更复杂的 RAG

而是：

- 一个可信的执行主链路
- 一个统一的控制面契约
- 一个可验证的审批 / 审计 / 回放闭环

### Explicit non-goals for this phase

以下能力在本阶段都不是主目标，只允许“保持可扩展接口”，不允许反过来主导架构：

- Slack / Web / 钉钉 Adapter 产品化
- Milvus 长记忆正式接线
- Wasm 技能全面接管
- OpenTelemetry 全链路生产化上报
- 多 Agent 团队协同高级编排

---

## North Star

构建以下唯一可信金路径：

`request -> validate -> route -> runtime -> tool/approval -> chunk -> final/error -> audit -> store`

只要这条链路没有完全可信，其他扩展能力一律视为后续插件。

---

## System invariants

以下不变量必须先于新功能成立：

### 1. Transport invariants

- 队列与广播必须是两个不同抽象
- 同一类消息只能有一种投递语义
- 同一类结果只能有一个统一通道名

### 2. Runtime invariants

- `context.Context` 取消必须快速终止运行循环
- 每个 `TaskID` 只能进入一个终态
- 超时、取消、审批拒绝必须可区分

### 3. Safety invariants

- 所有写操作审批都必须有稳定幂等键
- 审批回调必须验签、防重放、可超时
- 审批迟到决策只能审计，不能改终态

### 4. Event invariants

- 内部 chunk 与外部 SSE 使用同一事件模型
- 业务终止事件唯一
- SSE 传输终止事件唯一

### 5. Test invariants

- fake 与 real 的语义必须对齐
- 单元测试不能依赖 fake 的宽松行为
- 关键路径必须有取消、超时、幂等、迟到决策测试

---

## Workstreams

## Workstream 0: Documentation reset

### Outcome

让文档先对齐真实阶段，避免“平台叙事”继续压过“内核闭环”。

### Tasks

- [ ] 在 `docs/README.md` 中加入本重置计划入口
- [ ] 明确区分“当前执行目标”与“远期平台愿景”
- [ ] 标注尚未存在的文档为待补，而不是默认可用

### Files

- `docs/README.md`
- `docs/superpowers/plans/2026-03-18-runtime-governance-reset.md`

---

## Workstream 1: Split queue and event bus

### Outcome

把任务派发和事件广播从一个含混的 `PubSub` 抽象中拆开。

### Design direction

新增两个接口：

```go
type TaskQueue interface {
    Enqueue(ctx context.Context, queue string, msg bus.Message) error
    Dequeue(ctx context.Context, queue string) (<-chan bus.Message, error)
}

type EventBus interface {
    Publish(ctx context.Context, topic string, msg bus.Message) error
    Subscribe(ctx context.Context, topic string) (<-chan bus.Message, error)
}
```

### Rationale

- `tasks` / `task.<id>` / `task_results` 属于“任务或结果交付”
- `approvals` / `chunks.*` / `system.health` / heartbeat 属于“广播事件”

### Tasks

- [ ] 识别所有现有 channel 的语义类别
- [ ] 拆出 queue 与 event bus 两套接口
- [ ] 为 Redis 分别实现 queue 路径与 event 路径
- [ ] 为 fake 实现提供相同语义

### Files

- `internal/bus/client.go`
- `internal/bus/redis_pubsub.go`
- `internal/bus/client_test.go`
- `internal/bus/message.go`
- `cmd/orchestrator/main.go`
- `cmd/subagent/main.go`

### Acceptance

- [ ] 没有任何任务交付继续复用广播接口
- [ ] 没有任何广播事件继续伪装成 list 队列

---

## Workstream 2: Repair the gold path

### Outcome

让单任务单 Agent 的最小执行链路真正跑通。

### Gold path definition

1. 接收 `/v1/chat/completions`
2. 严格校验请求
3. 解析静态模型路由
4. 调用 Runtime
5. 如有工具则执行
6. 如为写操作则进入审批
7. 生成统一 chunk 事件
8. 返回 final / error
9. 落审计与任务状态

### Tasks

- [ ] 统一任务结果通道名
- [ ] 让 orchestrator 与 subagent 使用同一结果回收约定
- [ ] 确保 Runtime 返回的终态与状态存储一致
- [ ] 为单任务链路补充最小集成测试

### Files

- `cmd/orchestrator/main.go`
- `cmd/subagent/main.go`
- `internal/workflow/workflow.go`
- `internal/memory/sqlite_task_state_store.go`
- `cmd/orchestrator/main_test.go`
- `cmd/subagent/main_test.go`

### Acceptance

- [ ] orchestrator 能收到 subagent 的最终结果
- [ ] workflow 节点能从 running 进入 success / failed
- [ ] 单任务路径在 fake 与 real 语义下都成立

---

## Workstream 3: Runtime lifecycle hardening

### Outcome

让运行时在取消、超时、空队列、审批等待等场景下具有确定性退出行为。

### Tasks

- [ ] `Subagent.Run` 在 `ctx.Done()` 后快速返回
- [ ] fake consume channel 在取消时关闭
- [ ] heartbeat 语义明确化：启动即发送一次，随后周期发送
- [ ] 为取消、超时、无消息场景补测试

### Files

- `cmd/subagent/main.go`
- `cmd/subagent/main_test.go`
- `internal/bus/client.go`
- `internal/bus/client_test.go`
- `internal/bus/mqtt_heartbeat.go`

### Acceptance

- [ ] `go test ./cmd/subagent -timeout 20s` 稳定通过
- [ ] 不再存在因为消费 channel 永不关闭导致的测试卡死

---

## Workstream 4: Approval contract unification

### Outcome

把审批从“概念存在”升级为“真正可信的写操作门禁”。

### Design direction

统一使用：

- `TaskID`
- `ToolCallID`
- `TraceID`

作为审批相关操作的关联信息。

### Tasks

- [ ] 统一 webhook body 与 runtime 审批等待结构
- [ ] 审批回调接入 HMAC + timestamp + nonce 校验
- [ ] 明确拒绝、超时、迟到决策的状态转换
- [ ] 把审批决策纳入审计事件

### Files

- `internal/skill/approval_gate.go`
- `internal/skill/approval_gate_test.go`
- `internal/skill/webhook_auth.go`
- `internal/skill/webhook_auth_test.go`
- `internal/bus/message.go`
- `internal/llm/contracts.go`
- `cmd/orchestrator/main.go`

### Acceptance

- [ ] 每个审批决策都能映射到唯一 `TaskID + ToolCallID`
- [ ] 未验签的回调一律拒绝
- [ ] 迟到决策不会篡改已终结状态

---

## Workstream 5: Strict OpenAI-compatible ingress

### Outcome

让 API 入口真正遵守规格文档，而不是“看起来兼容”。

### Tasks

- [ ] HTTP handler 使用 `ValidateChatCompletionRequest`
- [ ] 对 `tool_choice`、`tools`、`temperature`、未知字段执行严格校验
- [ ] 错误映射与规格一致
- [ ] 增加入口层测试，避免只在包内 validator 测试通过

### Files

- `internal/gateway/chat_completions_handler.go`
- `internal/llm/openai_compat.go`
- `internal/llm/openai_compat_test.go`
- `internal/gateway/sse_handler_test.go`

### Acceptance

- [ ] 未知字段返回 `400 invalid_request_error`
- [ ] 非法模型别名返回统一错误
- [ ] 流式与非流式入口共用同一验证逻辑

---

## Workstream 6: Single event model for chunk + SSE

### Outcome

把内部 chunk 事件与外部 SSE 彻底统一到 `StreamChunk`。

### Tasks

- [ ] Runtime 在 think / tool_call / tool_result / final / error 时真正发出 chunk
- [ ] Sender 从事件总线订阅统一 chunk 事件
- [ ] SSE 仅做 `StreamChunk -> SSE frame` 的格式转换
- [ ] 保证 final/error 业务终止唯一、done 传输终止唯一

### Files

- `internal/llm/runtime_loop.go`
- `internal/llm/stream_fanout.go`
- `internal/llm/stream_fanout_test.go`
- `internal/gateway/sender.go`
- `internal/gateway/sender_test.go`
- `internal/gateway/sse_handler.go`
- `internal/gateway/chat_completions_handler.go`

### Acceptance

- [ ] 内部事件与外部 SSE 不再各走一套协议
- [ ] SSE 中断不会破坏业务终态
- [ ] chunk 序号严格单调递增

---

## Workstream 7: Audit as a first-class system

### Outcome

把审计从“写文件日志”升级为“结构化治理证据链”。

### Design direction

审计至少覆盖：

- route
- runtime_turn
- tool_call
- waiting_approval
- approval_decision
- tool_result
- final
- error
- cancelled
- timeout

### Tasks

- [ ] 引入统一 `AuditEvent` 结构
- [ ] 让 runtime / approval / gateway / orchestrator 写入同一审计模型
- [ ] 避免仅依赖本地 `audit.log`

### Files

- `internal/process/audit.go`
- `internal/llm/contracts.go`
- `cmd/orchestrator/main.go`
- `cmd/subagent/main.go`

### Acceptance

- [ ] 单个 `TaskID` 能串起完整证据链
- [ ] 审计事件可用于回放与排障

---

## Deferred capabilities

以下能力只做接口保留，不纳入本轮主线交付：

- [ ] `internal/memory/milvus_store.go` 的正式检索接线
- [ ] `internal/sandbox/wazero_executor.go` 的通用技能执行接线
- [ ] `pkg/telemetry/otel.go` 的生产级接线
- [ ] `internal/gateway/router.go` 的多渠道产品化适配
- [ ] 高级多 Agent 协作语法与父子任务图扩展

---

## Delivery sequence

### Week 1: harden control plane

- [ ] 拆 queue / event bus
- [ ] 修结果通道断链
- [ ] 修取消语义
- [ ] 修审批契约
- [ ] 接入入口 strict validation

### Week 2: close the gold path

- [ ] 打通单次请求到 final/error
- [ ] 打通单次工具调用
- [ ] 打通单次写操作审批
- [ ] 打通统一 chunk 事件
- [ ] 打通任务状态落库

### Week 3: make it provable

- [ ] 迟到审批测试
- [ ] SSE 中断测试
- [ ] cancel / timeout / max-turns 测试
- [ ] fake / real 语义对齐测试
- [ ] 单任务回放式审计测试

---

## Definition of done

本轮只有在满足以下条件时才算完成：

- [ ] `go test ./cmd/orchestrator ./cmd/subagent ./internal/... ./pkg/... -timeout 20s` 稳定通过
- [ ] `cmd/subagent` 不再出现取消测试挂死
- [ ] 写操作审批链路具备验签、防重放、幂等键
- [ ] `/v1/chat/completions` 真正执行 strict validation
- [ ] 内部 chunk 与 SSE 共用单一事件模型
- [ ] 单个任务可从请求一路追踪到终态和审计

---

## Final note

这个重置计划不是收缩 ambition，而是重建顺序。

先把 `agentic-core` 做成一个**可信的 Agent Runtime**，再把它扩展成一个**企业级多智能体平台**。  
如果顺序反过来，平台会放大不确定性；如果顺序正确，平台只是内核的自然延伸。
