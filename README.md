# Agentic-Core (企业级多智能体核心引擎) 🚀

Agentic-Core 是一个从零构建的、高度自治、防幻觉、企业级的 Multi-Agent 核心系统。旨在作为大型生产制造、Agentic ERP 或企业认知引擎的底层中枢。

## 🌟 核心愿景
与传统的基于 K8s 或 Docker 的编排不同，Agentic-Core 采用**最硬核的操作系统级进程控制 (`os/exec`)** 来实现动态调度，确保极高的响应速度和进程级隔离。原生支持 Agentic Workflow、长短记忆（RAG）与 Wasm 沙盒安全执行。

## 🛠️ 技术栈与架构红线
*   **核心语言**: Golang 1.21+ (Context 全程控制)。
*   **进程调度**: 标准库 `os/exec` 动态 Fork 子进程（Sub-agents）。
*   **消息总线**: `Redis Pub/Sub` 用于任务申领，`MQTT` 用于高频心跳状态监控。
*   **记忆中枢**: 
    *   **短记忆/执行状态**: `SQLite` (纯 Go 驱动 `glebarez/sqlite`)。
    *   **长记忆/RAG**: `Milvus` (高性能向量数据库)。
*   **沙盒执行**: 使用 WebAssembly (`wazero`) 进行代码级隔离执行。
*   **团队协作**: 支持 `@` 语法（如 `@researcher`）实现子任务分发与结果交付，主任务实时监控。

## 📁 目录结构
*   `cmd/orchestrator/`: 编排器（主控节点）入口，负责任务分发与状态维护。
*   `cmd/subagent/`: 子智能体进程入口，支持多种角色（Researcher, Coder 等）。
*   `internal/process/`: 子进程生命周期管理与 Agent 注册表（Registry）。
*   `internal/bus/`: Redis/MQTT 消息总线封装。
*   `internal/memory/`: SQLite 任务状态存储与 Milvus 长记忆检索。
*   `internal/workflow/`: 基于 DAG 的 Agentic Workflow 状态机。
*   `internal/sandbox/`: Wasm 沙盒执行逻辑。

## 👥 团队协作功能 (New!)
Agentic-Core 支持多智能体团队模式：
1.  **角色感知**: 每个 Agent 启动时均加载其特定的角色提示词（Role Prompt）。
2.  **子任务分发**: 主任务运行过程中，可以通过在 Payload 中使用 `@agent_name`（如：`@researcher 请帮我查阅...`）来动态请求其他专业智能体协助。
3.  **层级监控**: Orchestrator 会自动维护父子任务关系，主任务在 SQLite 中能随时查看到子任务的执行进度与交付结果。

## 🚀 快速启动

### 1. 基础设施
使用 Docker Compose 启动 Redis, MQTT 和 Milvus:
```bash
docker-compose up -d
```

### 2. 预检与演示
运行预检脚本确认环境：
```bash
bash scripts/preflight.sh
```

运行演示脚本观察多智能体协作：
```bash
bash scripts/team_demo.sh
```

## 🛡️ 安全与防幻觉机制
*   **强类型契约**: 所有的消息传递均基于严格定义的 Go struct 和 JSON Schema。
*   **HITL (Human-in-the-loop)**: 对于高风险写操作，主控会自动挂起并触发 Webhook 审批请求。

---
Developed by **nikkofu** | V1.0 Alpha
