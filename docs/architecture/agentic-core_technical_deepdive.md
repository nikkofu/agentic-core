# Agentic-Core: Agentic-Core 深度对标技术规格书 (V2)

## 1. 接入与路由层 (IM Gateway & Session Router)

### 1.1 Channel Adapter (渠道适配器)
*   **设计模式**: 适配器模式 (Adapter Pattern)。
*   **统一协议**: 定义 `internal/gateway/types.go` 中的 `StandardMessage`。
    *   `Source`: 来源渠道 (wechat, slack, web)。
    *   `ExternalID`: 外部消息唯一标识。
    *   `SessionID`: 转换后的标准会话 ID。
    *   `Content`: 消息内容或事件载荷。
*   **Gateway Sender**: 异步回传逻辑。监听 Redis 的 `outbox.[SessionID]` 频道，通过长连接或 API 推回至渠道。

### 1.2 Session Router (会话路由)
*   **状态维护**: 在 Redis 中维护 `session:active_agents` 哈希表。
*   **路由算法**: 
    1.  消息到达后，查询 SessionID 是否绑定了活跃 PID。
    2.  若绑定且 PID 进程存活，直接将消息转发至 `task.[TaskID]`。
    3.  若未绑定，调用 `Orchestrator.SpawnAgent` 创建专职智能体。

## 2. 核心引擎层 (Reasoning & Agentic Loops)

### 2.1 Model Resolver (模型解析器)
*   **兼容性**: 强制遵循 OpenAI Chat Completion V1 API。
*   **特性**:
    *   支持负载均衡 (Round-robin) 与 故障转移 (Failover)。
    *   支持 Token 计费与配额管理 (Quotas)。
    *   统一错误码映射。

### 2.2 System Prompt Builder
*   **动态渲染**: 使用 `text/template` 引擎。
*   **组件堆叠**:
    *   `Profile`: 核心人设。
    *   `TimeEnv`: 注入当前服务器时间、地点、操作系统等上下文。
    *   `MemoryBlock`: RAG 检索回来的相似片段。
    *   `ToolBlock`: 动态生成的 JSON Schema 工具描述。
    *   `Constraints`: 强化的防幻觉兜底指令。

### 2.3 Agentic Loops (ReAct 模式)
*   **状态机驱动**: 
    *   `IDLE` -> `THINKING` -> `ACTING` -> `OBSERVING` -> `IDLE` (或 `FINAL_ANSWER`)。
*   **Tool Call**: 解析 LLM 返回的 `tool_calls` 结构。
*   **Execute**: 调用 `internal/skill` 接口。对于 `IsWriteOperation: true` 的技能，进入 `WAITING_APPROVAL` 状态。

### 2.4 Stream Chunks (流式处理)
*   **协议**: 使用 `Server-Sent Events (SSE)` 兼容格式。
*   **内部流转**: LLM 返回的增量 Token 立即打包为 `MessageChunk` 推入 Redis，不等待完整生成。

## 3. 存储与上下文管理 (Memory & Context Guard)

### 3.1 Context Window Guard (防爆盾)
*   **Token 计数**: 引入 `tiktoken-go`。
*   **Compact 算法 (递归压缩)**:
    1.  当 `TotalTokens > MaxWindow * 0.8` 时启动。
    2.  保留最近的 2 轮对话作为原始引用。
    3.  调用“总结模型”将更早的消息归纳为 `Summary Memory`。
    4.  删除原始消息，将 `Summary Memory` 注入下一次请求的 `System Message`。

### 3.2 Session History
*   **分级存储**: 
    *   `L1 (Hot)`: Redis，存储当前对话上下文。
    *   `L2 (Warm)`: SQLite，存储近期的完整历史。
    *   `L3 (Cold)`: Milvus，对话片段向量化，用于跨会话的长记忆检索。

## 4. 可观测性 (Observability)

### 4.1 OpenTelemetry 注入
*   **TraceID 传递**: 在消息 `Message` 结构体中增加 `trace_id` 和 `span_id`。
*   **关键埋点**:
    *   `gateway.receive`
    *   `orchestrator.schedule`
    *   `llm.predict` (包含 Prompt Tokens / Completion Tokens)
    *   `skill.execute` (包含执行时长与结果状态)

### 4.2 审计日志 (Audit Logs)
*   记录每一轮 `Thought -> Action -> Observation` 的完整证据链，作为企业合规与防幻觉审查的依据。

---

## 5. 执行规划 (Action Plan)

| 优先级 | 模块 | 关键动作 |
| :--- | :--- | :--- |
| **P0** | **Reasoning Loop** | 将 `subagent` 改造成 `for` 循环的 ReAct 状态机。 |
| **P0** | **Tool Execution** | 实现真正的 `Tool Call` 解析与 `internal/skill` 的对接。 |
| **P1** | **Gateway Adapter** | 建立 `cmd/gateway` HTTP 服务，支持 cURL 接入。 |
| **P1** | **Context Guard** | 实现基于 LLM 的 `Compact` 摘要压缩逻辑。 |
| **P2** | **Observability** | 集成 `otel` SDK，开始上报全链路 Tracing。 |

---
**设计者**: Agentic-Core Architecture Team
**最后更新**: 2026年3月17日
