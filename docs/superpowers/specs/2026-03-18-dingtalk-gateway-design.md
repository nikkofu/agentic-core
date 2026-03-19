# DingTalk Gateway 接入设计

## 目标

在现有 `internal/gateway` 统一收发模型之上，补齐中国站 DingTalk（钉钉开放平台）接入能力，首版同时覆盖两条通道：

- `dingtalk_app`：企业内部应用 HTTP 事件回调 + 服务端 API 回发
- `dingtalk_robot`：群机器人 webhook 主动发送

并支持以下统一能力：

- 接收：文本、图片、音频、视频、文件、事件、卡片/互动回调
- 回复：文本、Markdown、图片、音频、视频、文件、卡片
- 路由：严格走现有 Gateway → Queue → Orchestrator → `task_results` → Adapter 回传链路
- 扩展：为后续更多通知型 / 会话型 IM 通道复用统一消息模型与 direct-send 能力

## 范围与边界

### 本次范围

1. 中国站 DingTalk，不包含国际化多地域域名切换
2. 企业内部应用的 HTTP 回调接入
3. 企业内部应用的服务端消息发送
4. 群机器人 webhook 发送
5. 统一消息结构中新增卡片承载字段
6. Gateway 增强“无需历史 route binding 的直接发送”能力
7. 为互动卡片 / 卡片回调预留统一承载与事件映射能力

### 非目标

1. 不在首版引入 Stream Mode / 长连接事件消费
2. 不实现钉钉完整消息类型全集之外的高级对象，如工作通知、审批实例、日历等业务对象
3. 不实现多租户凭证中心或多机器人 profile 管理系统
4. 不改动 orchestrator / subagent 的推理协议，仅扩展 Gateway 出站解释能力
5. 不重写现有 `internal/gateway/wecom/*`
6. 不为钉钉单独引入一套平行于 `gateway.StandardMessage` 的私有消息总线协议

## 调研结论

### 1. SDK 选型

钉钉官方开放平台提供服务端 SDK 下载入口，首版实现遵循“**尽量使用官方 SDK**”原则。但结合当前仓库结构与用户已确认的“HTTP 回调模式”，最佳拆分方式是：

1. **`dingtalk_app` 优先使用官方 Go SDK / OpenAPI client**
   - 当前落地采用“SDK + OAPI wrapper”混合方式
   - access token 获取与交互卡片发送走官方 Go SDK
   - 普通会话消息、工作通知与媒体上传走旧开放平台 OAPI wrapper，以覆盖当前 SDK 未完全封装的企业应用发送能力

2. **HTTP 回调验签 / 解密采用官方协议兼容的仓内轻量适配层**
   - 钉钉 HTTP 回调与服务端发消息是两类协议关注点
   - 即使引入官方 SDK，回调入口、统一消息映射、Gateway 路由衔接仍需仓内适配
   - 因此回调层应保持轻量、明确、可测试，并支持加密回调验签、解密和加密 `success` ACK

3. **`dingtalk_robot` 使用标准库轻量实现**
   - 机器人 webhook 与 app access token 生命周期解耦
   - 首版只需要 webhook URL、可选签名 secret、文本/卡片/媒体映射
   - 标准库实现更贴合当前仓库的轻依赖风格

### 2. 最终建议

采用“**官方 SDK + 仓内轻量适配层**”方案：

- `dingtalk_app`：优先使用官方 Go SDK，并在普通消息发送 / 媒体上传上补充 OAPI 调用
- `dingtalk_app` HTTP 回调：使用仓内 handler + mapper 适配 Gateway
- `dingtalk_robot`：使用标准库实现 webhook client
- Gateway 统一层负责抽象 `ChannelRequest` / `ChannelResponse`

原因：

- 满足用户要求，优先走官方 SDK 路线
- 与当前 `wecom` / `feishu` 的 Adapter 结构一致，便于并行协作
- 能把平台差异收敛在 `internal/gateway/dingtalk/*` 内部，减少对共享层的侵入

## 总体架构

新增 `internal/gateway/dingtalk/` 目录，内部拆分为以下组件：

- `config.go`：DingTalk app/robot 子配置与默认值
- `app_adapter.go`：企业应用适配器，实现 `gateway.RichAdapter`，并承载 HTTP 事件回调与互动卡片回调 handler
- `app_mapper.go`：事件 / 消息 / 卡片回调向统一消息模型的映射
- `app_client.go`：基于官方 SDK / OpenAPI client 的发送封装
- `robot_adapter.go`：群机器人适配器，实现 `gateway.RichAdapter`
- `robot_client.go`：机器人 webhook 发送、签名、payload 构造

同时对通用 Gateway 层做小幅增强：

- `internal/gateway/types.go`：新增卡片字段与更通用的原始内容承载
- `internal/gateway/router.go`：新增 direct-send 分支
- `internal/gateway/config.go`：从“按 `wecom` 固定校验”扩展到“按启用的 adapter 分别校验”
- `cmd/gateway/main.go`：按配置选择性注册 `wecom` / `wecom_robot` / `dingtalk_app` / `dingtalk_robot`

## 统一消息模型设计

在现有 `gateway.StandardMessage` 上做加法扩展，不破坏 `wecom` 已有行为：

- 保留：
  - `SessionID`
  - `ChannelName`
  - `MessageID`
  - `ParentMessageID`
  - `SenderID`
  - `SenderName`
  - `ReceiverID`
  - `Text`
  - `MessageType`
  - `Format`
  - `Media`
  - `Articles`
  - `Metadata`
- 新增：
  - `Card map[string]interface{}`：统一承载卡片 JSON
  - `RawContent json.RawMessage`：保留平台原始消息体，避免映射时丢字段

统一消息语义约定：

- `dingtalk_app` 回调进来的会话消息优先以 `conversationId` 作为 `SessionID`
- `conversationId`、`chatbotUserId`、`senderStaffId`、`senderNick`、`msgId`、`eventType`、`conversationType`、`cardCallbackId` 放入 `Metadata`
- 卡片交互回调统一映射为 `MessageTypeEvent`
- 卡片回传更新优先通过 `Card` 字段承载，而不是把 JSON 压进 `Text`

## 路由设计

当前 `SessionRouter.StartStreamListener()` 只能处理“先有入站 route binding，再有结果回推”的链路；这对通知型 webhook 通道不够，因为机器人发送本身不依赖先收到一条同构入站消息。

本次新增两个出站模式：

### 1. Route-bound reply（保留现有模式）

适用于：

- `wecom`
- `dingtalk_app`
- 后续其他会话型 IM app 通道

行为：

1. `HandleIncoming()` 入队前记录 `task_id -> route binding`
2. `task_results` 回来后查 binding 得到 `channel_name/session_id`
3. 若输出是 legacy `{"result":"..."}`，按文本回推
4. 若输出是 `{"message":{...}}`，按富消息结构回推

### 2. Direct-send reply（新增模式）

适用于：

- `dingtalk_robot`
- 未来任意通知型 webhook 通道

行为：

1. 若 `task_results` 中包含 `message.channel_name`
2. 且存在对应 adapter
3. 则无需 route binding，直接调用 adapter 发送

这样 Gateway 能同时支持：

- 会话型 IM（靠 route binding）
- 通知型 webhook（靠 direct-send）

### `task_results` direct-send 契约

当 orchestrator 希望 Gateway 直接发消息，而不是依赖已有入站 route binding 时，输出应采用：

```json
{
  "message": {
    "channel_name": "dingtalk_robot",
    "session_id": "",
    "message_type": "text",
    "text": "任务执行完成",
    "metadata": {
      "dingtalk_robot_webhook_url": "https://oapi.dingtalk.com/robot/send?access_token=demo"
    }
  }
}
```

关键约定：

- `message.channel_name` 必填，用于定位 adapter
- direct-send 不要求存在 `task_id -> route binding`
- `session_id` 对通知型 webhook 可为空；是否需要目标地址由 adapter 自行从配置或 `metadata` 决定
- 如果同时存在 route binding 且 `message.channel_name` 也存在，优先使用显式 `message.channel_name`

## DingTalk 企业应用设计

### 入站：HTTP 事件回调

接入两个 HTTP path：

- `DINGTALK_APP_EVENT_CALLBACK_PATH`：事件 / 消息订阅回调
- `DINGTALK_APP_CARD_CALLBACK_PATH`：互动卡片 / 卡片动作回调

事件处理策略：

1. 读取请求体并完成必要的验签 / 解密
2. 从回调体中提取：
   - 文本消息
   - 图片消息
   - 音频消息
   - 视频消息
   - 文件消息
   - 事件消息
   - 卡片交互事件
3. 映射为 `gateway.ChannelRequest`
4. 投递到 `SessionRouter.HandleIncoming()`
5. HTTP handler 同步返回平台要求的成功响应；后续消息通过异步出站发送

### 会话标识规则

- 首选 `conversationId` 作为 `SessionID`
- 若事件缺少 `conversationId`，回退到 `chatbotUserId` 或其他稳定 sender 标识
- 出站优先以会话维度标识作为接收目标
- `senderStaffId`、`robotCode`、`openConversationId` 等字段放入 `Metadata`，避免丢失平台特有上下文

### 出站：服务端消息发送

`dingtalk_app` 首版支持：

- 文本消息
- Markdown 消息
- 图片消息
- 音频消息
- 视频消息
- 文件消息
- 卡片消息

发送策略：

1. 根据 `ChannelResponse` 判断消息类型
2. 构造官方 SDK / OpenAPI request
3. 根据 `SessionID` 或 `Metadata` 中的接收目标字段填充接收对象
4. 根据消息类型调用 SDK 或 OAPI wrapper 发送消息

素材处理策略：

- 如果 `MediaItem.MediaID` 已经存在，直接发送
- 如果只有 `Path` 或 `DataBase64`，先上传媒体再发送
- 首版不做媒体缓存池或去重缓存
- 若平台只允许特定媒体类型走上传后发送，则在 adapter 内部完成映射，不向上层暴露平台差异

### 入站媒体处理

对入站图片 / 音频 / 视频 / 文件：

- 优先在 `Media` 中保留平台 `media_id` / 下载标识
- 若平台协议允许服务端二次下载，可参考 `wecom` 模式补充本地下载能力
- 首版以“保留可回传所需标识”为主，不强制在回调时完成大文件落盘

### 互动卡片回调

卡片回调统一映射为事件消息：

- `MessageType = event`
- `Text = "card_action"`
- `Card` 中保留完整卡片动作体的关键信息
- `Metadata` 中附带动作值、卡片实例标识、触发人、会话标识、事件时间等字段

这样 orchestrator 可以把它当作普通入站任务处理，而不需要另起专有协议。

## DingTalk 机器人设计

### 定位

`dingtalk_robot` 是一个通知型 adapter，不承诺接收用户消息，也不依赖 route binding。它的主要职责是把 Agentic-Core 的结果、告警、摘要、审批提醒以文本、Markdown、卡片形式发进钉钉群。

### 配置

首版支持一个默认 webhook，并允许通过消息 `metadata` 覆盖具体 webhook：

- `DINGTALK_ROBOT_ENABLED`
- `DINGTALK_ROBOT_WEBHOOK_URL`
- `DINGTALK_ROBOT_SECRET`

运行时覆盖约定：

- 若 `metadata.dingtalk_robot_webhook_url` 存在且非空，优先使用它
- 若 `metadata.dingtalk_robot_secret` 存在且非空，优先使用它
- 若消息体和默认配置都缺 webhook URL，则返回错误

### 发送能力

首版支持：

- 文本
- Markdown
- 卡片

说明：

- 机器人协议与 app API 并不完全同构
- 首版优先保证文本、Markdown、卡片稳定
- 图片 / 音频 / 视频 / 文件当前统一返回显式错误，不做静默降级

## 配置设计

在共享 `internal/gateway/config.go` 中新增：

- `DingTalkAppConfig`
- `DingTalkRobotConfig`

建议环境变量：

- `DINGTALK_APP_ENABLED`
- `DINGTALK_APP_CLIENT_ID`
- `DINGTALK_APP_CLIENT_SECRET`
- `DINGTALK_APP_AGENT_ID`
- `DINGTALK_APP_EVENT_CALLBACK_PATH`
- `DINGTALK_APP_CARD_CALLBACK_PATH`
- `DINGTALK_APP_API_BASE_URL`
- `DINGTALK_APP_OAPI_BASE_URL`
- `DINGTALK_APP_AES_KEY`
- `DINGTALK_APP_TOKEN`
- `DINGTALK_APP_MEDIA_DIR`
- `DINGTALK_APP_CARD_TEMPLATE_ID`
- `DINGTALK_APP_CARD_CALLBACK_ROUTE_KEY`
- `DINGTALK_ROBOT_ENABLED`
- `DINGTALK_ROBOT_WEBHOOK_URL`
- `DINGTALK_ROBOT_SECRET`

配置解析逻辑统一放到 `internal/gateway/config.go`，`cmd/gateway` 只负责启动与注册。

## 并行协作与冲突规避

当前仓库中另有 Codex 正在推进 `wecom` / `feishu` 相关工作，因此 DingTalk 实现必须明确约束写入边界：

- 不重写 `internal/gateway/wecom/*`
- 所有 DingTalk 平台逻辑都放入新增 `internal/gateway/dingtalk/*`
- 共享层只做最小必要改动：
  - `internal/gateway/types.go`
  - `internal/gateway/router.go`
  - `internal/gateway/config.go`
  - `cmd/gateway/main.go`
  - `cmd/gateway/.env.example`
- 共享层改动优先向 `feishu` 方案靠齐：
  - 统一新增 `Card`
  - 统一新增 `RawContent`
  - 统一 direct-send reply 语义

如果飞书分支在合并前已完成共享层演进，DingTalk 分支应以其为基础做增量接入，避免各自演化出两套不兼容的 `gateway.StandardMessage`。

## 测试策略

建议最小测试面如下：

- `internal/gateway/config_test.go`
  - 验证 DingTalk env + flag 解析
  - 验证 app-only / robot-only / mixed mode 的校验逻辑
- `internal/gateway/router_test.go`
  - 验证 route-bound reply
  - 验证 direct-send reply
  - 验证 metadata 合并与 channel 定位
- `internal/gateway/dingtalk/app_handler_test.go`
  - 验证 URL challenge / 回调成功响应
  - 验证文本 / 图片 / 音频 / 视频 / 文件 / 事件 / 卡片动作映射
  - 验证异常 body / 验签失败 / 映射失败的状态码
- `internal/gateway/dingtalk/app_client_test.go`
  - 验证文本 / Markdown / 卡片发送
  - 验证媒体上传后发送
  - 验证缺少接收目标时返回显式错误
- `internal/gateway/dingtalk/robot_client_test.go`
  - 验证默认 webhook 发送
  - 验证 `metadata` 覆盖 webhook / secret
  - 验证文本 / Markdown / 卡片 payload
  - 验证不支持媒体类型时返回明确错误

## 非功能要求

- 所有平台字段映射必须显式、可测试，不允许把大段未解释 JSON 混进 `Text`
- 所有 webhook / 回调 handler 必须返回明确状态码，不能吞掉错误
- 所有 token / secret / callback body 日志必须避免泄漏敏感信息
- 不为首版引入额外重型框架；优先使用标准库 + 官方 SDK

## 结论

本次 DingTalk 接入的最优方案是：

- `dingtalk_app` 走 **HTTP 回调模式**
- `dingtalk_app` 的服务端 API **尽量使用官方 SDK**
- `dingtalk_robot` 走 **webhook 轻量实现**
- Gateway 共享层补齐 **卡片承载字段** 与 **direct-send reply**

这样既满足用户要求，又能最大限度复用当前 `gateway` 架构，并在与 `wecom` / `feishu` 并行开发的前提下，把 DingTalk 的改动边界控制在可管理范围内。
