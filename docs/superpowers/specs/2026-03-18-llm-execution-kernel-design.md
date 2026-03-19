# LLM 执行内核设计规格（MVP）

- 日期：2026-03-18
- 主题：LLM Execution Kernel（OpenAI-compatible / Static Routing / Dual Streaming / Full Tool Execute）
- 适用仓库：`agentic-core`

## 1. 背景与目标

在现有 `agentic-core` 框架下，优先落地“LLM 执行内核”第一子项目，作为后续 IM Gateway、Channel Adapter、Memory、Observability 全面扩展的核心引擎。

本规格聚焦以下已确认范围：

1. OpenAI-compatible 协议
2. 模型静态路由（不做动态探测）
3. SSE + 内部 chunk 双通道
4. Tool Execute 三件套：内置工具 + HITL 审批 + WASM 沙箱

### 1.1 OpenAI-compatible 兼容矩阵（规范性约束）

本 MVP 仅实现以下兼容面：

- **HTTP Endpoint**：`POST /v1/chat/completions`
- **请求字段（支持）**：`model`, `messages`, `tools`, `tool_choice`, `temperature`, `stream`
- **请求字段（暂不支持）**：`response_format` 的高级 schema 模式、`parallel_tool_calls` 等扩展字段
- **响应结构（非流式）**：遵循 Chat Completions 形态，包含 `choices[].message` 与可选 `tool_calls`
- **响应结构（流式）**：SSE 帧格式 `event: <type>\ndata: <json>\n\n`，传输终止帧为 `event: done` + `data: [DONE]`
- **错误映射**：
  - 路由缺失/配置错误 -> `400`（`invalid_request_error`）
  - 上游鉴权失败 -> `401`（`authentication_error`）
  - 上游限流 -> `429`（`rate_limit_error`）
  - 上游超时/网关失败 -> `502/504`（`api_error` / `timeout_error`）

参数级契约（严格模式）：

| 字段 | 类型 | 允许值/范围 | 默认值 | 非法值处理 |
| --- | --- | --- | --- | --- |
| `model` | string | 非空；必须命中静态路由表 alias | 无 | `400 invalid_request_error` |
| `messages` | array | 至少 1 条；`role` ∈ {system,user,assistant,tool} | 无 | `400 invalid_request_error` |
| `tools` | array | 仅 function 类型 | 空数组 | `400 invalid_request_error` |
| `tool_choice` | string/object | `auto`/`none`/`required`/指定 function | `auto` | `400 invalid_request_error` |
| `temperature` | number | `[0,2]` | provider 默认值 | `400 invalid_request_error` |
| `stream` | bool | `true/false` | `false` | `400 invalid_request_error` |

未知字段策略：默认 strict 模式，出现未知字段返回 `400 invalid_request_error`。

兼容策略说明：MVP 以“请求/响应主干兼容”为目标，不承诺覆盖 OpenAI 全量参数。

## 2. 架构分层与模块边界

### 2.1 Model Resolver
- 职责：将逻辑模型名映射到具体 provider endpoint/key/model。
- 约束：静态映射、强配置驱动、缺失即报错。

### 2.2 LLM Runtime（ReAct Loop Driver）
- 职责：统一驱动 `THINK -> TOOL -> OBSERVE -> FINAL`。
- 约束：必须有 `MaxTurns`、超时与失败退出机制。

### 2.3 Tool Orchestrator
- 三层执行：
  - Builtin Executor
  - Approval Gate（`IsWriteOperation=true`）
  - WASM Executor（wazero）
- 统一接口，Runtime 不感知执行后端差异。

### 2.4 Stream Fanout
- 单一 chunk 事件模型同时用于：
  - Redis 内部事件
  - SSE 外部流
- 保证序号单调递增与可重放。

### 2.5 Session & Context Guard
- 会话历史：SQLite（结合现有 session store）。
- 上下文防爆：阈值触发 compact，总结旧历史，保留最近轮次原文。

### 2.6 Observability & Audit
- 记录完整证据链：think、route、tool_call、approval、execute、stream、final。
- TraceID 贯穿消息、执行、日志与回放。

## 3. 关键数据契约

### 3.1 InferenceRequest
```go
type ChatMessage struct {
    Role       string          `json:"role"`
    Content    string          `json:"content"`
    Name       string          `json:"name,omitempty"`
    ToolCallID string          `json:"tool_call_id,omitempty"`
    ToolCalls  json.RawMessage `json:"tool_calls,omitempty"`
}

type InferenceRequest struct {
    TraceID      string
    SessionID    string
    TaskID       string
    AgentType    string
    Messages     []ChatMessage // 与 chat/completions messages 对齐，保序
    UserInput    string        // 可选派生字段，便于审计检索
    ModelAlias   string
    MaxTurns     int
    Stream       bool
    Metadata     map[string]string
    OnApprovalReject string // continue | fail
}
```

映射规则：
- `/v1/chat/completions` 的 `messages` 原样保序映射到 `InferenceRequest.Messages`
- `UserInput` 仅从最后一个 user message 派生，不参与协议还原

### 3.2 StaticRoute
```go
type StaticRoute struct {
    Alias         string
    Provider      string
    BaseURL       string
    APIKeyRef     string
    UpstreamModel string
    TimeoutMs     int
}
```

### 3.3 StreamChunk
```go
type StreamChunk struct {
    TraceID      string
    SessionID    string
    TaskID       string
    Sequence     int64
    Event        string // delta | tool_call | tool_result | final | error | heartbeat | done
    Delta        string
    Role         string
    ToolName     string
    Done         bool
    Error        string
    Data         json.RawMessage // 按 Event 承载结构化 payload
    TimestampMs  int64
}
```

事件字段约束：

- `delta`：必须包含 `Delta`
- `tool_call`：`Data` 必须可反序列化为 `ToolCall`
- `tool_result`：`Data` 必须可反序列化为 `ToolResult`
- `final`：业务终止事件，`Done=true` 且 `Data` 包含最终文本/结构化结果
- `error`：业务终止事件，`Done=true` 且 `Error` 非空
- `done`：传输终止事件（仅 SSE 输出强制），对外发送 `[DONE]`

唯一性约束：
- **业务终止唯一性**：同一 `TaskID` 只能出现一次 `final` 或 `error`
- **传输终止唯一性**：同一 SSE 连接只能发送一次 `done:[DONE]`

### 3.4 ToolCall / ToolResult
```go
type ToolCall struct {
    ID               string          `json:"id"`
    Name             string          `json:"name"`
    Arguments        json.RawMessage `json:"arguments"`
    IsWriteOperation bool            `json:"is_write_operation"`
}

type ToolResult struct {
    ToolCallID   string `json:"tool_call_id"`
    Name         string `json:"name"`
    Success      bool   `json:"success"`
    Output       string `json:"output"`
    Error        string `json:"error"`
    DurationMs   int64  `json:"duration_ms"`
}
```

校验规则：
- `ToolCall.Name` 必填
- `Arguments` 必须为有效 JSON 对象
- 写操作判断统一使用 `IsWriteOperation`，禁止别名字段。

### 3.5 ApprovalRequest / ApprovalDecision
```go
type ApprovalRequest struct {
    TraceID       string
    TaskID        string
    ToolCallID    string
    ToolName      string
    Arguments     json.RawMessage
    RequestedAtMs int64
}

type ApprovalDecision struct {
    TraceID       string
    TaskID        string
    ToolCallID    string
    Approved      bool
    Reviewer      string
    Reason        string
    DecidedAtMs   int64
}
```

### 3.6 AuditEvent
```go
type AuditEvent struct {
    TraceID      string
    TaskID       string
    SessionID    string
    Stage        string
    Status       string // ok | error | waiting_approval | rejected
    Summary      string
    Payload      json.RawMessage
    TimestampMs  int64
}
```

## 4. ReAct 状态机

状态集合：

- `INIT`
- `BUILD_PROMPT`
- `LLM_THINK`
- `PARSE_MODEL_OUTPUT`
- `TOOL_DISPATCH`
- `WAITING_APPROVAL`
- `TOOL_EXECUTE`
- `OBSERVE_APPEND`
- `STREAM_FINAL`
- `DONE`
- `FAILED`
- `CANCELLED`
- `TIMEOUT`
- `ABORTED_STREAM`

状态转换规则：

1. `LLM_THINK` 返回 final -> `STREAM_FINAL` -> `DONE`
2. `LLM_THINK` 返回 tool_call -> `TOOL_DISPATCH`
3. `TOOL_DISPATCH` 遇写操作 -> `WAITING_APPROVAL`
4. 审批通过 -> `TOOL_EXECUTE`
5. 审批拒绝 -> 依据策略 `OnApprovalReject`：
   - `continue`：将拒绝 observation 回注后回到 `LLM_THINK`
   - `fail`：直接进入 `FAILED`
6. 审批超时 -> `TIMEOUT`
7. 任意轮工具结果写入 observation 后回到 `LLM_THINK`
8. 达到 `MaxTurns` 且无 final -> `FAILED`
9. 上下文取消（`context.Context` canceled）-> `CANCELLED`
10. Provider 超时 -> `TIMEOUT`
11. SSE sink 失败但内部 chunk 成功时 -> `ABORTED_STREAM`
    - 若业务已产生 `final/error`：`ABORTED_STREAM -> DONE`
    - 若业务未终止且后续执行失败/超时：`ABORTED_STREAM -> FAILED/TIMEOUT`

幂等与终止约束：
- 审批回调以 `ToolCallID + TaskID` 作为幂等键；重复回调仅首条生效。
- 超时后到达的审批回调标记为 `ignored_late_decision`，只审计不改变状态。
- 并发冲突（approve/reject 同时到达）采用持久化 CAS “先写入成功者生效”。
- 每个 `TaskID` 只允许一个终态（`DONE`/`FAILED`/`CANCELLED`/`TIMEOUT`）。

## 5. 目录落位与增量实施

## 5.1 目录落位

- `internal/llm/`
  - `resolver.go`
  - `stream.go`
  - `runtime_loop.go`
  - `schema_parser.go`
  - `context_guard.go`（增强）
- `internal/skill/`
  - `registry.go`
  - `executor_builtin.go`
  - `approval_gate.go`
  - `executor_wasm.go`
- `internal/session/`
  - `history_store.go`（复用）
  - `context_compactor.go`
- `internal/gateway/`
  - `sse_handler.go`
  - `sender.go`
- `internal/process/`
  - `audit.go`（增强）

## 5.2 里程碑顺序

### Milestone 1：核心闭环（不含审批/WASM）
- 静态路由 + ReAct loop + 双通道流式 + 只读 builtin 工具。

### Milestone 2：HITL 审批
- 写工具进入 `WAITING_APPROVAL`，Webhook 决策恢复/拒绝。

### Milestone 3：WASM 工具
- 接入 wazero 执行器，tool_result 回注与流式事件同步输出。

### Milestone 4：Context + Audit 增强
- compact 生效，TraceID 全链路一致，审计证据链完整。

## 6. 测试与验收

### 6.1 单测
- 路由映射成功/缺失
- 模型输出解析正确/异常
- 状态机循环、max_turn 终止
- 工具执行成功/失败
- 审批通过/拒绝/超时
- chunk 顺序与 final 终止语义

### 6.2 集成
- E2E：只读工具链路
- E2E：写工具审批链路
- E2E：WASM 工具链路
- E2E：长历史 compact 链路
- 协议黄金测试：OpenAI-compatible 请求/响应与 SSE 帧快照校验
- 审批乱序/重复回调测试：仅首条决策生效，迟到回调标记 ignored
- 流式慢消费者/断连测试：SSE 失败不影响内部 chunk
- Provider 4xx/5xx/timeout 映射测试
- 审批 Webhook 鉴权测试：HMAC 签名、timestamp 窗口、nonce 防重放
- 敏感字段脱敏测试：审计日志不得泄露密钥与高敏参数
- 异常恢复测试：重启/重试后 sequence 单调与终止事件不重复

### 6.3 DoD
- OpenAI-compatible 静态路由可用
- ReAct loop 稳定可控
- SSE + 内部 chunk 双通道可消费
- 内置工具 + HITL + WASM 全部跑通
- compact 可触发并回注
- 审计日志可还原完整执行证据链
- 关键契约测试（协议/审批幂等/流式容错/鉴权防重放）通过

## 7. 非目标（MVP 不做）

- 动态智能选路
- 多租户计费系统
- 全渠道 adapter 一次性接入
- 高级 RAG rerank 与策略编排

## 8. 风险与控制

- **配置漂移风险**：静态路由配置集中化，启动时校验。
- **循环失控风险**：强制 `MaxTurns` + per-turn timeout。
- **写操作越权风险**：审批门前置，拒绝即终止写执行（或按策略回注）。
- **流式一致性风险**：统一 chunk 模型与序列号生成器。
- **安全与合规风险**：审计日志对密钥/敏感参数做脱敏；Tool 参数与结果支持 PII 屏蔽策略。
- **审批回调安全风险**：Webhook 必须做 HMAC 签名校验 + 时间窗口校验 + nonce 防重放。
- **运维风险**：SSE/内部事件设置 TTL、重放窗口和缓冲上限，防止无限积压。
- **序列唯一性风险**：`Sequence` 在 `TraceID+TaskID` 作用域内单调唯一，重启后不复用终止事件。

---
本规格对应子项目：`LLM 执行内核`，用于后续 implementation plan 编排与分阶段开发。
