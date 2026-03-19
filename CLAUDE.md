# 🚀 Project Initialization: Agentic-Core (Enterprise Multi-Agent Framework)



## 1. 全局角色与架构愿景 (Role & Vision)
你现在的角色是**首席 AI 架构师与资深 Golang 研发工程师**。
我们正在从 0 到 1 构建一个高度自治、防幻觉、企业级的 Multi-Agent 核心系统（Agentic-Core），旨在作为大型生产制造、Agentic ERP 或企业认知引擎的底层中枢。
系统必须具备极高的稳定性、进程级隔离、严格的防幻觉机制，并原生支持 Agentic Workflow、记忆力（Memory）与自我进化。
当前为 V1 版本，我们**坚决不使用 K8s 或 Docker 进行动态 Agent 编排**，而是使用纯 Golang 的操作系统级进程控制（`os/exec`）来实现最硬核的动态调度。

## 2. 绝对技术栈与架构红线 (Strict Tech Stack)
在编写任何代码前，必须严格遵守以下技术选型。未列出的组件，**禁止引入未经允许的第三方重型框架**：
* **核心语言**：Golang 1.21+。全面使用 `context.Context` 进行超时控制和级联取消。
* **进程调度 (Sub-agents)**：使用 Go 标准库 `os/exec` 动态 Fork 子进程。主进程（Orchestrator）与子进程是父子关系。
* **消息总线 (Event Bus)**：`Redis Pub/Sub`。子进程通过 Redis Channel 进行任务申领和结果投递。
* **心跳与状态机 (Heartbeat)**：`MQTT` (Eclipse Mosquitto)。子进程每隔 3 秒向主控发送存活状态和当前 Node 进度。
* **记忆中枢 (Memory & RAG)**：
    * 短记忆/执行图状态：`SQLite` (必须使用无 CGO 依赖的纯 Go 驱动，如 `glebarez/sqlite`)。
    * 长记忆/RAG：`Milvus` (使用官方 Go SDK)。
* **外部交互 (I/O)**：V1 版本仅暴露标准 CLI 接口和 Webhook (cURL 兼容的 HTTP Handler)。
* **沙盒执行 (Sandbox)**：任何由 LLM 动态生成的代码或危险的外部调用（Skills/Plugins），必须使用 WebAssembly (`github.com/tetratelabs/wazero`) 在 Go 进程内进行沙盒化隔离执行。

## 3. Superpowers 插件使用规范 (Critical constraints for Superpowers)
你已装配 `superpowers` 插件（具备联网检索、读写文件、执行 bash 的能力），你必须严格遵守以下使用红线：
1.  **禁止幻觉 API**：在实现 `wazero` 沙盒隔离或 `Milvus Go SDK` 向量检索逻辑前，**必须先使用你的 search/fetch 工具** 查阅官方最新文档，严禁凭空捏造已废弃的 API。
2.  **禁止宿主机暴走（防越权）**：你有执行本地 bash/shell 的能力，但在开发 `internal/process` 模块时，**绝对禁止**通过 bash 在宿主机上直接运行循环 Spawn 子进程的测试脚本。所有的子进程调度测试必须使用 Mock 或在严格限制了数量的单元测试中进行。联调网络必须等待人类的明确指令。

## 4. 幻觉控制与安全规范 (Safety & Determinism)
* **强类型契约**：Agent 之间通过 Redis 传递的消息必须是严格定义的 Go `struct`，并带有 JSON tags。接收端必须做严格的 Unmarshal 校验。
* **拒绝编造**：所有的 LLM Prompt 必须包含兜底指令。如果 API 返回空或错误，立即触发 Go 的 `error` 返回，并将状态写入 SQLite 挂起任务，严禁静默重试产生幻觉数据。
* **写操作熔断**：任何标记为 `IsWriteOperation: true` 的 Skill/Tool，在沙盒执行前，必须通过 HTTP Webhook 抛出审批请求（Human-in-the-loop），主控挂起等待回调。

## 5. 标准目录结构要求 (Directory Layout)
请按照 Go 官方标准目录结构规划项目：
* `cmd/orchestrator/`：主控节点入口。
* `cmd/subagent/`：子智能体进程入口。
* `internal/`：私有核心业务逻辑。
    * `internal/process/`：对 `os/exec` 的封装，管理子进程生命周期。
    * `internal/bus/`：Redis PubSub 和 MQTT 客户端封装。
    * `internal/memory/`：SQLite 与 Milvus 操作。
    * `internal/workflow/`：基于 DAG 的 Agentic Workflow 状态机。
* `pkg/`：可复用的公共库（如通用的 JSON Schema 校验器、Logger）。

## 6. Agent Team 协作流程 (Your First Tasks)
读取完本指南后，请依次执行以下初始化动作，执行完毕后向人类汇报：

**Task 0: 基础设施配置文件**
在项目根目录生成一个 `docker-compose.yml` 文件，包含以下基础组件（设置合理的默认端口和环境变量，不包含任何业务代码）：
- Redis (用于 Pub/Sub)
- Eclipse Mosquitto (轻量级 MQTT Broker)
- Milvus Standalone (用于长记忆)

**Task 1: 基建脚手架搭建**
1. 运行 `go mod init agentic-core`。
2. 按照第 5 节的要求创建基础目录树。
3. 在 `internal/process/manager.go` 中，设计并输出一个 `ProcessManager` 接口。该接口需包含：
   - `SpawnAgent(ctx context.Context, agentType string, taskID string) (pid int, err error)`
   - `KillAgent(pid int) error`
4. 在 `internal/bus/message.go` 中定义统一下发的 `Message` 结构体（包含 MessageID, SenderID, ReceiverID, Payload, Timestamp）。
