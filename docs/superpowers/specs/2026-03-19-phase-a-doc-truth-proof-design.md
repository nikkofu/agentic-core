# Phase A 文档真相与 Gold-Path Proof 设计规格

- 日期：2026-03-19
- 主题：Phase A — Documentation Truth Reset + Gold-Path Proof Suite
- 适用仓库：`agentic-core`

## 1. 背景

当前仓库已经形成了比早期平台叙事更明确的近期主线：先把 `agentic-core` 收敛成 **Agent Runtime + Governance Kernel**，优先打通“可验证、可解释、可复跑”的执行主链路，再扩展更大的平台能力。

这一点已经在以下文档中明确：

- `docs/superpowers/plans/2026-03-18-runtime-governance-reset.md`
- `docs/roadmap/task_backlog.md`
- `docs/superpowers/specs/2026-03-18-llm-execution-kernel-design.md`

但仓库当前仍存在两个阻碍近期主线推进的问题：

1. **文档真相漂移**
   - `README.md` 仍用“企业级多智能体核心引擎”“原生支持 RAG / Wasm / 团队协作”等叙事作为主入口。
   - `docs/README.md` 已开始指向 reset/backlog，但仍有缺失文档链接未补齐。
   - 新进入项目的人容易把 deferred 能力误判成当前主线能力。

2. **Gold-path 证据分散**
   - 关键回归测试其实已经覆盖了审批成功、审批拒绝、审批超时与迟到决策、SSE 中断、审计回放、路由回传等重要场景。
   - 但这些证据散落在多个测试文件里，没有一个对人和 LLM 都清晰的统一入口。
   - 结果是仓库“已经有证据”，但没有形成“可复跑、可讲清、可作为后续设计边界”的 proof suite。

Phase A 的目标不是增加新的核心运行能力，而是把**仓库的真实阶段**与**仓库的可证明能力**先统一起来。

## 2. 目标与非目标

### 2.1 目标

Phase A 只做两件事：

1. **BG-001：Documentation Truth Reset**
   - 纠正根文档与文档入口的能力表述。
   - 补齐当前已经被索引但尚不存在的关键占位/说明文档。
   - 让新贡献者能快速知道：什么已实现、什么部分实现、什么明确延后。

2. **BG-103：Gold-Path Proof Suite Consolidation**
   - 将现有核心回归证据整理成一套可命名、可复跑、可解释的 proof bucket。
   - 提供统一文档入口与命名脚本入口。
   - 增补一条轻量的 real-transport 或 scripted smoke slice，补足 fake-heavy 测试的盲区。

### 2.2 非目标

本阶段**不**做以下工作：

- 不新增审计持久化表与查询接口
- 不新增 sticky session 持久路由表
- 不重构 gateway / orchestrator / runtime 的主业务逻辑
- 不扩展 provider 控制面
- 不把 WASM 接入当前主线
- 不引入新的宏大路线图文档体系
- 不重写整仓所有历史文档

Phase A 是 **truth + proof**，不是新的核心功能开发阶段。

## 3. 当前事实与设计原则

### 3.1 当前事实

当前仓库已具备以下 gold-path 证据基础：

- 审批成功后写工具完成
- 审批拒绝后状态持久化
- 审批超时与迟到决策忽略
- SSE 中断时的传输终止审计
- replay 风格的终态审计保真
- gateway 入站与回传路由的统一负载

同时，当前仓库仍存在以下 Phase A 应直接解决的问题：

- `README.md` 仍把远期平台能力写得像当前能力
- `docs/README.md` 依赖的若干链接尚未落盘
- gold-path 证明路径没有一个统一、显式、可命名的运行入口

### 3.2 设计原则

1. **先修入口，不扩大战场**
   - 只修最关键的文档入口与缺失锚点，不做整仓文档重排。

2. **先组织证据，不重复造测试**
   - 先把已存在的强回归测试组织成 proof suite，再只补最少量 smoke。

3. **对未来 Phase B 保持解耦**
   - proof 文档和脚本不预绑定未来 audit/session 持久化表结构，避免后续返工。

4. **让人类和 LLM 都能快速理解**
   - 文档命名、章节与脚本输出都要清楚表达“证明的是什么”和“没有证明什么”。

## 4. Phase A 范围切分

### 4.1 为什么先做 BG-001 和 BG-103

近期主线真正要推进的后续工作是：

- `BG-101` Durable audit evidence chain
- `BG-102` Sticky session routing

但在推进这两项之前，必须先把仓库的“说法”和“证据”整理干净。否则后续实现会在两个层面持续发散：

- 团队继续被旧的“平台先行”叙事牵引
- 已经存在的 gold-path 证据无法被明确引用，导致重复测试与重复争论

因此，Phase A 明确聚焦：

- **文档 truth reset**
- **proof suite consolidation**

而将 durable evidence chain 和 sticky routing 留到 Phase B 一并推进。

### 4.2 Phase A 交付边界

Phase A 交付完成后，仓库应该满足：

- 对外文档入口不再高估当前能力
- 缺失文档链接全部可解析
- gold-path 证据拥有统一文档入口
- gold-path 证据拥有统一命名脚本入口
- proof bucket 的边界、覆盖面和局限性都被明确写出

## 5. 文档真相重置设计

### 5.1 根入口文档

`README.md` 应从“平台愿景首页”收敛为“当前 runtime phase 首页”。

它需要明确分成三层：

1. **当前已实现 / 可证明的能力**
   - OpenAI-compatible `/v1/chat/completions` 主链路
   - 严格请求校验
   - 静态模型路由
   - 审批门禁与签名回调
   - unified chunk / SSE fanout
   - task terminal states
   - gateway channel slices
   - unified logging foundation

2. **部分实现能力**
   - 审计事件已存在但 evidence chain 尚未持久化成第一类能力
   - session routing 已存在但仍是单进程内路由
   - session/context/history/tool execution 仍在 hardening 阶段

3. **明确延后能力**
   - 生产级 RAG / Milvus 主链路
   - 完整的多 Agent 用户语义
   - WASM 作为当前主线执行后端
   - 生产级 OTEL / metrics

### 5.2 文档中心入口

`docs/README.md` 继续作为文档导航中心，但需要升级为“当前执行锚点入口”：

- 强化 reset plan、backlog、当前 active specs 的优先级
- 新增当前状态矩阵：
  - implemented
  - partial
  - deferred
- 明确指出某些文档是 placeholder / current-state notes，而不是完整设计总集

### 5.3 缺失链接补齐

补齐以下文件，使当前文档入口不再出现悬空链接：

- `docs/architecture/process_scheduling.md`
- `docs/protocol/internal_bus.md`
- `docs/journal/README.md`

这些文档的第一版目标不是“覆盖全部历史细节”，而是做到：

- 链接有效
- 说明当前事实
- 指出真实 source-of-truth
- 明确哪些内容尚待后续补充

## 6. Gold-Path Proof Suite 设计

### 6.1 总体思路

proof suite 采用“双层入口”：

1. **说明文档层**
   - 面向人和 LLM
   - 解释每个 proof bucket 在证明什么
   - 解释覆盖边界与未覆盖边界

2. **命名脚本层**
   - 面向执行
   - 按 bucket 稳定运行对应测试
   - 输出清晰、可终端阅读、可被日志/LLM 消费的结果摘要

### 6.2 新增文件

建议新增：

- `docs/testing/gold_path_proof.md`
- `scripts/proof_gold_path.sh`

### 6.3 为什么采用脚本而不是 Makefile

当前仓库已经存在 `scripts/` 目录与多个脚本入口，但没有统一的 `Makefile` 约定。

因此，Phase A 采用脚本而不是引入新的构建入口，原因是：

- 与现有仓库习惯一致
- 更适合逐桶执行与输出清晰日志
- 不引入新的维护约定

## 7. Proof Bucket 设计

### 7.1 Bucket 列表

proof suite 至少包含以下 bucket：

- `approval-success`
- `approval-reject`
- `approval-timeout-late-decision`
- `audit-replay`
- `sse-abort`
- `gateway-route`
- `smoke`
- `all`

### 7.2 Bucket 到现有测试映射

#### `approval-success`

证明：

- 写工具在审批通过后能够继续执行
- gateway 与 orchestrator 的审批通过主链路都可成立

映射到现有测试：

- `internal/gateway/chat_completions_handler_test.go` 中的 `TestChatCompletionsHandlerExecutesWriteToolAfterApproval`
- `cmd/orchestrator/main_test.go` 中的 `TestServeHTTPCompletesWriteToolAfterApprovalWebhook`

#### `approval-reject`

证明：

- 审批拒绝会被保留为一等治理结果
- 拒绝不会被静默压缩成一般失败

映射到现有测试：

- `internal/gateway/chat_completions_handler_test.go` 中的 `TestChatCompletionsHandlerRejectsWriteToolApprovalAndPersistsRejectedState`

#### `approval-timeout-late-decision`

证明：

- 审批等待超时会进入 `timeout`
- 超时后的迟到审批只审计，不改终态

映射到现有测试：

- `internal/gateway/chat_completions_handler_test.go` 中的 `TestChatCompletionsHandlerTimeoutDuringApprovalPersistsTimeoutAndIgnoresLateDecision`
- `cmd/orchestrator/main_test.go` 中的 `TestServeHTTPLateApprovalWebhookDoesNotChangeTimedOutWriteToolState`

#### `audit-replay`

证明：

- task replay 风格的审计回放能保留终态与错误信息

映射到现有测试：

- `cmd/orchestrator/main_test.go` 中的 `TestSingleTaskReplayAuditPreservesTerminalResultStatus`

#### `sse-abort`

证明：

- SSE sink 中断时不会伪造错误 done 帧
- aborted stream 会留下明确的审计证据
- 终态 chunk 与传输终止的关系被正确保留

映射到现有测试：

- `internal/gateway/sender_test.go` 中的 `TestSenderDisconnectDoesNotEmitDoneFrame`
- `internal/gateway/sender_test.go` 中的 `TestSenderPublishesAbortedStreamAuditOnDisconnect`
- `internal/gateway/sender_test.go` 中的 `TestSenderPublishesDoneStatusAuditWhenFinalChunkDisconnects`

#### `gateway-route`

证明：

- gateway 入站负载被统一封装
- route binding 可以驱动回传适配器
- direct-send 与 binding merge 语义成立

映射到现有测试：

- `internal/gateway/router_test.go` 中的 `TestHandleIncomingTracksTaskRouteAndEnqueuesUnifiedPayload`
- `internal/gateway/router_test.go` 中的 `TestStartStreamListenerRoutesRichMessageToRegisteredAdapter`
- `internal/gateway/router_test.go` 中的 `TestStartStreamListenerDirectSendUsesExplicitChannelMessage`
- `internal/gateway/router_test.go` 中的 `TestStartStreamListenerDirectSendOverridesRouteBinding`

### 7.3 `smoke` bucket 的定位

`smoke` bucket 不是第二套完整集成测试体系，而是一条轻量、现实 transport 或脚本驱动的补充证明链，用来回答：

> “当前仓库的主线信心，是否完全依赖 fake transport？”

Phase A 对 `smoke` bucket 的要求是最小可用：

- 尽量复用现有脚本体系
- 不引入复杂的 CI 新基建
- 不扩展为渠道全矩阵联调
- 必须至少跨过一个真实系统边界，而不是完全停留在内存 fake 中

可接受的 `smoke` 例子包括：

- 启动一个本地 HTTP handler，真实走一遍 `/v1/chat/completions` 或统一 ingress，并验证终端输出 / SSE 输出
- 在本地可用依赖存在时，走一次真实 Redis-backed queue/event transport 的最小主链路

不接受的 `smoke` 例子：

- 只是把现有单元测试重新换个名字执行
- 全程仍只依赖 in-memory fake transport、fake sender、fake route map，而没有跨越任何真实边界

它的作用是补盲区，而不是喧宾夺主。

## 8. 运行入口与输出约定

### 8.1 脚本接口

`scripts/proof_gold_path.sh` 建议支持：

```bash
bash scripts/proof_gold_path.sh approval-success
bash scripts/proof_gold_path.sh approval-reject
bash scripts/proof_gold_path.sh approval-timeout-late-decision
bash scripts/proof_gold_path.sh audit-replay
bash scripts/proof_gold_path.sh sse-abort
bash scripts/proof_gold_path.sh gateway-route
bash scripts/proof_gold_path.sh smoke
bash scripts/proof_gold_path.sh all
```

### 8.2 输出约定

脚本输出需要同时满足三类可读性：

1. **终端可读**
   - 每个 bucket 开始/结束都有清晰标题
   - 每条命令打印前后有上下文
   - 最终有 PASS/FAIL 汇总

2. **日志友好**
   - 失败时能快速定位是哪个 bucket、哪条命令失败
   - 结果摘要应适合收集到日志中

3. **LLM 友好**
   - bucket 名称稳定
   - 说明与输出结构简洁明确
   - 避免把多个不相关测试混成“一个大命令黑盒”

## 9. 验收标准

Phase A 完成后，应满足以下验收标准：

### 9.1 文档验收

- `README.md` 不再把 deferred 能力写成已落地能力
- `docs/README.md` 提供清晰的当前状态导航
- `docs/architecture/process_scheduling.md`、`docs/protocol/internal_bus.md`、`docs/journal/README.md` 全部存在且内容可用
- 新贡献者在 5 分钟内能找到：
  - 当前 source-of-truth
  - 当前 backlog
  - gold-path proof 文档
  - proof 运行入口

### 9.2 Proof 验收

- `docs/testing/gold_path_proof.md` 清楚描述各 bucket 的证明边界
- `scripts/proof_gold_path.sh all` 能显式运行整套 proof buckets
- bucket 输出可读、可复跑、可汇总
- `smoke` bucket 至少补足一条非纯 fake 的主线证明

### 9.3 组织效益验收

- 团队后续讨论 `BG-101` 与 `BG-102` 时，可以直接引用 proof buckets
- 不再需要在多个测试文件之间手工拼接“项目现在到底能证明什么”

## 10. 风险与控制

### 10.1 风险：文档范围蔓延

如果把 Phase A 变成全仓文档改造，会拖慢近期主线。

控制方式：

- 只修入口与缺失锚点
- 不在本阶段重写全部旧文档

### 10.2 风险：proof suite 变成重复造轮子

如果把 proof suite 理解成“新增一套大而全的测试体系”，会浪费已有成果。

控制方式：

- 以现有测试映射为主体
- 只补缺口驱动的 smoke slice

### 10.3 风险：过早耦合 Phase B 实现

如果在 Phase A 就预埋 durable audit / sticky session 的具体数据模型，会使后续设计失去弹性。

控制方式：

- proof 文档与脚本只绑定事实行为和运行命令
- 不提前绑定未来存储结构

## 11. 向 Phase B 的过渡

Phase A 完成后，Phase B 将自然围绕两个问题展开：

1. **哪些 gold-path 事件已经明确需要 durable evidence chain**
   - approval
   - chunk
   - final / error / timeout / cancelled / rejected
   - replay / terminal correlation

2. **哪些 session / route 行为已经证明有价值，但仍停留在单进程内实现**
   - session binding
   - in-flight route ownership
   - stale binding expiry
   - late-result handling

这意味着 Phase A 不是独立文档清理动作，而是为以下下一阶段提供稳定边界：

- `BG-101` Durable audit evidence chain
- `BG-102` Sticky session routing

## 12. 实施摘要

Phase A 的实现应围绕以下文件进行：

**新增：**

- `docs/testing/gold_path_proof.md`
- `docs/architecture/process_scheduling.md`
- `docs/protocol/internal_bus.md`
- `docs/journal/README.md`
- `scripts/proof_gold_path.sh`

**修改：**

- `README.md`
- `docs/README.md`

本阶段完成后，仓库将具备一个更诚实的入口叙事，以及一套可命名、可复跑、可作为后续治理设计边界的 gold-path proof suite。
