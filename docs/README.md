# Agentic-Core 文档中心

欢迎来到 Agentic-Core 的核心设计与规划中心。本项目致力于构建企业级的分布式多智能体引擎。

## 📖 文档导航

### 1. 战略与规划
*   [Master Roadmap](./enterprise_roadmap_agentic-core.md): 项目总体演进蓝图与阶段目标。
*   [Runtime Governance Reset Plan](./superpowers/plans/2026-03-18-runtime-governance-reset.md): 项目再定位与 P0 / P1 / P2 整改计划。
*   [Task Backlog](./roadmap/task_backlog.md): 详细的功能拆解与开发积压任务。

### 2. 架构深度设计
*   [Agentic-Core Architecture](./architecture/agentic-core_technical_deepdive.md): 对标 Agentic-Core 的技术细节规格书。
*   [Process & Scheduling](./architecture/process_scheduling.md): 进程级调度与隔离机制说明。
*   [Gateway WeCom Runbook](./gateway_wecom_runbook.md): 企业微信 Gateway、自建应用回调、统一 JSON 入站、群机器人 webhook 联调手册。
*   [Gateway Feishu Runbook](./gateway_feishu_runbook.md): 飞书 Gateway、自建应用事件回调、卡片回调、群机器人 webhook 与 direct-send 联调手册。

### 3. 通信协议
*   [Internal Bus Protocol](./protocol/internal_bus.md): Redis/MQTT 消息定义与状态机说明。

### 4. 开发日志 (Journal)
*   [Development Journal Index](./journal/README.md): 每日工作小结与决策记录。

---

## 🛠️ 工程规范
*   **路径规范**: 所有文档、脚本、代码中严禁使用绝对路径。必须使用相对项目根目录的路径。
*   **提交规范**: 遵循 Angular Commit 规范。
*   **文档同步**: 每次重大功能更新后，必须同步更新对应的 Markdown 文档。

## 🪵 日志运行参数
*   **默认目录**: `logs/YYYY-MM-DD/<service>.jsonl`
*   **终端输出**: 默认开启，使用人类可读文本格式
*   **文件输出**: 默认开启，使用 JSON Lines 结构化格式
*   **日志级别**: `LOG_LEVEL`，支持 `debug` / `info` / `warn` / `error`，默认 `info`
*   **日志目录**: `LOG_DIR`，默认 `logs`
*   **保留天数**: `LOG_RETENTION_DAYS`，默认 `30`

## ⚠️ 当前状态说明
*   当前文档中心中的部分链接仍指向待补文档；在补齐前，请优先以 `docs/superpowers/plans/2026-03-18-runtime-governance-reset.md` 与 `docs/superpowers/specs/2026-03-18-llm-execution-kernel-design.md` 作为近期执行锚点。
