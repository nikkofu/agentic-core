# 任务终态语义对齐设计规格（MVP）

- 日期：2026-03-19
- 主题：Task Terminal State Alignment
- 适用仓库：`agentic-core`

## 1. 背景与问题

当前仓库已经在两条关键链路上逐步形成了较好的治理语义：

1. `gateway -> llm runtime -> tool/approval -> chunk -> final/error`
2. `orchestrator -> subagent -> task_results -> task store -> audit`

但这两条链路对“任务终态”的表达仍不一致：

- `gateway` 已能区分 `timeout`、`cancelled`、`rejected`、`failed`
- `subagent` 与 `orchestrator` 侧仍会在部分路径中把这些状态压缩或归并
- `task store` 注释与测试心智仍偏向 `pending/running/success/failed`

这会直接削弱以下能力：

- Human-in-the-loop 审批的治理价值
- Replay / Audit 的可解释性
- 终态统计与重试策略的准确性
- “失败”与“超时/取消/人工拒绝”的责任边界

本设计的目标不是新增 LLM 能力，而是把**已经存在的终态语义，真正贯通成一个统一契约**。

## 2. 目标与非目标

### 2.1 目标

在现有架构下统一任务终态表达，使以下层保持一致：

- `llm.FinalResult.Status`
- `bus.TaskResult.Status`
- `memory.TaskState.Status`
- `llm.AuditEvent.Status`

并保证：

- `timeout` 不再被压成 `failed`
- `cancelled` 不再被压成 `failed`
- `rejected` 不再被压成 `failed`
- 未知旧状态仍有保守兜底

### 2.2 非目标

本阶段**不**做以下扩展：

- 不新增数据库表或迁移 SQLite schema
- 不扩展 `/v1/chat/completions` 对外协议
- 不引入新的 `failure_class` / `terminal_reason` 字段
- 不把 workflow 内部状态枚举扩成多终态状态机
- 不重构审计系统或回放系统的数据模型

## 3. 统一状态模型

### 3.1 合法状态集合

本阶段统一采用以下任务状态集合：

- `pending`
- `running`
- `success`
- `failed`
- `rejected`
- `timeout`
- `cancelled`

语义说明：

| 状态 | 含义 |
| --- | --- |
| `pending` | 已创建、尚未开始执行 |
| `running` | 正在执行中 |
| `success` | 正常完成 |
| `failed` | 业务失败、执行失败、解析失败或未知失败 |
| `rejected` | 需要审批的写操作被人工拒绝 |
| `timeout` | 运行超时或审批等待超时 |
| `cancelled` | 上下文取消导致终止 |

### 3.2 关键约束

- `failed` 不能再作为所有异常终态的统称
- `rejected`、`timeout`、`cancelled` 必须被视为一等终态
- 任何未知状态输入都只能在**单一收口点**被归一化为 `failed`
- 审计必须尽量保留原始终态，而不是先归一化再记录

## 4. 模块职责与边界

### 4.1 LLM Runtime

`internal/llm` 负责生成尽可能精确的终态：

- `success`
- `failed`
- `timeout`
- `cancelled`
- `rejected`

运行时不负责数据库兼容，也不负责历史状态迁移。

### 4.2 Subagent

`cmd/subagent` 的职责是**保真透传**运行时终态：

- 运行时提供了终态时，原样写入 `TaskResult.Status`
- 运行时未给终态时，才做最小兜底

Subagent 不再自行创造额外的压缩语义。

### 4.3 Orchestrator

`cmd/orchestrator` 是任务状态收口层：

- 校验并规范化 `TaskResult.Status`
- 保持 `success/failed/rejected/timeout/cancelled` 原样落库
- 未知状态在这里统一兜底成 `failed`

Orchestrator 负责兼容旧输入，但不应吞并新语义。

### 4.4 Task Store

`internal/memory` 的职责是**保存事实状态**，不负责推断语义。

- `TaskState.Status` 继续使用字符串
- 注释、测试和调用方心智更新为完整状态集合
- 不修改 SQLite schema

### 4.5 Workflow

`internal/workflow` 本阶段继续维持“编排态”而不是“治理态”：

- `success -> completed`
- 其余终态 -> failed

原因：

- Workflow 关心 DAG 是否继续推进
- TaskState / Audit 关心为什么没推进

这样可避免一次性扩大 workflow 改造面。

## 5. 状态流转设计

### 5.1 Runtime 到 Subagent

`llm.Runtime.Run` 返回 `FinalResult.Status` 后：

- `subagent` 读取该状态
- 构造 `bus.TaskResult.Status`
- 将状态原样放入 `task_results`

如果运行时未提供状态：

- `err != nil` -> `failed`
- `err == nil` -> `success`

### 5.2 Subagent 到 Orchestrator

`orchestrator` 消费 `task_results` 后：

1. 读取 `TaskResult.Status`
2. 用统一状态契约做合法化/归一化
3. 保存到 `memory.TaskState.Status`
4. 保留原始 payload 到审计 `Data`

### 5.3 Orchestrator 到 Workflow

Workflow 映射保持简单：

- `success` -> `MarkCompleted`
- `failed/rejected/timeout/cancelled` -> `MarkFailed`

这里的“失败”只代表 DAG 节点未成功完成，不代表丢失原始治理语义。

### 5.4 Gateway 路径

`gateway` 已具备更精细的任务状态推断能力。

本阶段设计要求：

- 不再让 `gateway` 单独维护另一套状态枚举心智
- 后续实现时复用同一个小型状态契约
- 确保 gateway 路径与 orchestrator/subagent 路径的终态表意一致

## 6. 契约收口设计

建议新增一个小型内部状态契约层，负责：

- 定义合法状态集合
- 判断某状态是否为终态
- 归一化未知状态
- 提供运行时/任务结果/存储路径的共享帮助函数

建议边界：

- 放在 `internal/memory` 或一个更中性的轻量位置
- 不引入对 `gateway`、`workflow`、`subagent` 的反向耦合

契约能力应尽量简单，避免过早抽象成复杂状态机。

建议至少提供：

```go
func NormalizeTaskStatus(status string) string
func IsTerminalTaskStatus(status string) bool
func IsSuccessfulTaskStatus(status string) bool
```

如需从错误推导状态，可提供独立 helper，但不强制各层都依赖错误字符串推断。

## 7. 错误处理与兼容策略

### 7.1 未知状态兼容

当接收到旧代码或异常路径送来的未知状态值时：

- 落库状态归一化为 `failed`
- 审计 payload 保留原始状态值
- 不因未知状态直接丢弃结果消息

### 7.2 终态优先级

本阶段不引入复杂的终态冲突解决规则，默认：

- 任务结果消息以最后接收到的合法结果为准
- 但当前代码路径预期每个任务只应上报一次最终结果

如果未来需要终态幂等控制，应在更高层单独设计。

### 7.3 审计语义

审计应尽量保真：

- `timeout` 就记录 `timeout`
- `rejected` 就记录 `rejected`
- `cancelled` 就记录 `cancelled`

避免先压缩为 `failed` 再写审计。

## 8. 影响文件

预计主要涉及以下文件：

- `cmd/orchestrator/main.go`
- `cmd/orchestrator/main_test.go`
- `cmd/subagent/main.go`
- `cmd/subagent/main_test.go`
- `internal/memory/task_state_store.go`
- `internal/memory/sqlite_task_state_store.go`
- `internal/gateway/chat_completions_handler.go`
- `internal/gateway/chat_completions_handler_test.go`

视实现切分情况，可能新增一个轻量状态契约文件，例如：

- `internal/memory/task_status.go`
- `internal/memory/task_status_test.go`

## 9. 测试设计

### 9.1 契约单测

验证：

- 合法状态通过
- 未知状态归一化为 `failed`
- `success/failed/rejected/timeout/cancelled` 终态判断正确

### 9.2 Subagent 单测

验证：

- `runtime` 返回 `timeout` 时，`TaskResult.Status == timeout`
- `runtime` 返回 `cancelled` 时，`TaskResult.Status == cancelled`
- `runtime` 返回 `rejected` 时，`TaskResult.Status == rejected`

### 9.3 Orchestrator 单测

验证：

- 收到 `timeout` 结果后，`taskStore` 保存 `timeout`
- 收到 `rejected` 结果后，`taskStore` 保存 `rejected`
- 收到 `cancelled` 结果后，`taskStore` 保存 `cancelled`
- 收到未知状态时，`taskStore` 保存 `failed`

### 9.4 审计兼容单测

验证：

- 原始 `task_results` payload 中的状态仍保留在审计事件 `Data`
- 归一化仅影响落库状态，不影响审计证据链

## 10. 风险与取舍

### 10.1 为什么不直接扩展 Workflow 状态

因为这会把本次目标从“治理语义对齐”扩大成“编排状态机重构”。

当前更合理的分工是：

- Workflow：只判断节点是否成功推进
- TaskState / Audit：解释节点为何未推进

### 10.2 为什么不新增数据库字段

因为当前 `status` 已是字符串，足够承载本次对齐需求。

在未证明现有状态字段不足之前，不应为了“理论完美”引入 schema 迁移。

### 10.3 为什么不现在引入 `failure_class`

这是下一阶段可以考虑的增强方向，但当前最重要的是先让已有终态语义不再丢失。

先把事实状态保真，再讨论更细的失败分类，符合“先做硬约束，再做高能力”的项目主线。

## 11. 验收标准

满足以下条件即可视为本设计完成落地：

- `timeout/rejected/cancelled` 在 subagent → orchestrator → task store 链路中不再被压成 `failed`
- gateway 路径与 orchestrator/subagent 路径使用同一状态心智
- 旧未知状态输入仍可兼容，且安全归一化为 `failed`
- 审计继续保留原始终态证据
- 相关单测覆盖全部关键状态分支

## 12. 后续演进方向

本设计完成后，后续可以继续演进：

1. 引入更细的 `failure_class` / `terminal_reason`
2. 为 replay / dashboard 增加终态维度统计
3. 在 workflow 层引入更细的治理型节点结果

但这些都应建立在本次“统一终态语义”已经稳定之后。
