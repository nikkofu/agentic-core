# Agentic-Core (Agentic-Core-Compatible) 企业级架构设计方案

## 1. 愿景与目标 (Vision)
构建一个高度自治、防幻觉、企业级的 Multi-Agent 系统。参考 Agentic-Core 的成熟模式，将 `agentic-core` 的硬核进程调度（os/exec）与现代大模型应用的网关层、会话管理层、推理循环层深度融合，作为企业级认知引擎的底层中枢。

## 2. 核心模块详解 (Core Components)

### 2.1 接入层 (Gateway & Channel Layer)
*   **Channel Adapter (渠道适配器)**: 
    *   **职责**: 统一不同 IM 平台（微信、Slack、Web Widget、钉钉）的数据协议。
    *   **输入**: 各平台原始 Webhook Payload。
    *   **输出**: 标准化的 `ChannelRequest`。
*   **IM Gateway (接入网关)**: 负责管理长连接（WebSocket）或 HTTP 回调，处理鉴权与频率限制。
*   **Gateway Sender (回传器)**: 监听内部总线的 `stream_chunks` 或 `final_response`，将其反序列化并推送回原始渠道。

### 2.2 路由层 (Routing Layer)
*   **Session Router (会话路由器)**: 
    *   **逻辑**: 将 `(ChannelName, UserID/GroupID)` 映射为内部唯一的 `SessionID`。
    *   **功能**: 如果会话已有活跃的 Agent 进程，则将消息路由至对应的 Redis Channel；否则，触发 Orchestrator 创建新 Agent。
*   **Message Queue (消息队列)**: 核心通信基于 `Redis LPUSH/BRPOP`。
    *   `tasks`: 待分配的原始任务。
    *   `task.[TaskID]`: 特定 Agent 的指令流。
    *   `task_results`: 任务完成后的结果汇总。

### 2.3 核心引擎层 (Reasoning Engine)
*   **Model Resolver (模型解析器)**: 兼容 OpenAI 格式。负责处理不同模型（GPT-4, Claude 3, DeepSeek, Ollama）的 API 差异，并管理 API Key 轮询与负载均衡。
*   **System Prompt Builder (动态提示词构建器)**: 
    *   **动态组装**: `Base Role` + `Current Context (Time/Env)` + `Retrieved Memory (RAG)` + `Tool/Skill Schemas` + `Session History`。
*   **Agentic Loops (ReAct/Plan-Execute)**: 
    *   **循环逻辑**: `Think -> Act -> Observe`。
    *   **多轮推理**: LLM 根据 Observation 修正下一步行动，直到得出 Final Answer。
*   **Tool Call & Execute**: 
    *   **解析**: 捕获 LLM 的 `call_skill` 意图。
    *   **执行**: 在 `wazero` 沙盒或本地白名单中运行 Skill。
    *   **返回**: 将结果注入下一次推理循环。

### 2.4 存储与上下文层 (Memory & Context Layer)
*   **Session History (会话历史)**: 存储于 SQLite。记录每一轮对话的角色、内容、Token 数、时间戳。
*   **Context Window Guard (上下文守卫)**: 
    *   **Compact (压缩机制)**: 当历史长度超过模型窗口（如 8k/128k）的 70% 时，触发“历史总结（Summarization）”，将早期消息转化为一段摘要存储在 System Prompt 中，从而腾出空间。
*   **Memory (RAG)**: 对接 Milvus。自动将对话片段向量化，供后续检索使用。

### 2.5 观测与日志层 (Observability)
*   **OpenTelemetry**: 注入 TraceID。追踪从 IM 消息进入 -> 网关转发 -> Orchestrator 调度 -> Agent 推理 -> Wasm 调用的全链路耗时。
*   **Stream Chunks (流式输出)**: 实时推送 LLM 的生成片段，降低首字延迟。
*   **Logs (审计日志)**: 详细记录 Agent 的 `think` 过程（思考链），用于防幻觉分析与合规检查。

## 3. 技术栈选型 (Strict Tech Stack)
*   **IM Adapter**: Gin/Echo (Golang HTTP Framework)。
*   **Protocol**: JSON + Protocol Buffers (针对高性能内部通信)。
*   **Token Counting**: `tiktoken-go` 或类似实现。
*   **Sandbox**: `github.com/tetratelabs/wazero`。
*   **Tracing**: Jaeger / OTEL Collector。

## 4. 演进路线图 (Milestones)

### Phase 1: Gateway & Basic Session (当前进行中)
*   [x] 基础 `SessionRouter` 逻辑。
*   [x] SQLite `SessionHistory` 存储。
*   [x] 简单的 `ContextGuard` 截断逻辑。

### Phase 2: Agentic Loop & Tool Use (下一阶段)
*   [ ] 实现 `Plan-Execute` 循环逻辑。
*   [ ] 对接 OpenAI `tools` 字段。
*   [ ] 集成内置的 HTTP/Time Skills。

### Phase 3: RAG & Observability (进阶阶段)
*   [ ] 接入 Milvus 长记忆检索。
*   [ ] 注入 OpenTelemetry 全链路追踪。
*   [ ] 实现历史 Compact (递归摘要) 逻辑。

### Phase 4: Channel Adapters (产品化阶段)
*   [ ] 编写微信、Slack、钉钉的官方 Adapter。
*   [ ] 实现 `Gateway Sender` 的流式回传。

---
**设计者**: Agentic-Core Architecture Team
**最后更新**: 2026年3月17日
