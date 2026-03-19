# Feishu Gateway 接入设计

## 目标

在现有 `internal/gateway` 统一收发模型之上，补齐中国站 Feishu（飞书开放平台）接入能力，首版同时覆盖两条通道：

- `feishu_app`：自建应用事件订阅 + 服务端 API 回发
- `feishu_bot`：群机器人 webhook 主动发送

并支持以下统一能力：

- 接收：自建应用消息事件、消息卡片回调事件
- 回复：文本、Markdown、图片、文件、交互卡片
- 路由：严格走现有 Gateway → Queue → Orchestrator → `task_results` → Adapter 回传链路
- 扩展：为后续 `lark` 国际站与更多 IM 适配器保留统一消息模型与直接发送能力

## 范围与边界

### 本次范围

1. 中国站 Feishu，不包含国际站 `larksuite` 域名与凭证切换
2. 自建应用事件订阅接入
3. 自建应用消息发送
4. 自定义机器人 webhook 发送
5. 统一消息结构中新增卡片承载字段
6. Gateway 增强“无需历史 route binding 的直接发送”能力

### 非目标

1. 不实现国际站 `lark` 多域名兼容
2. 不实现飞书完整消息类型全集；首版只覆盖文本、Markdown、图片、文件、卡片，以及必要的事件消息映射
3. 不实现文件下载、上传素材缓存、媒体转存等高级能力
4. 不改动 orchestrator / subagent 的推理协议，仅扩展 Gateway 出站解释能力
5. 不引入多机器人 profile 管理系统；`feishu_bot` 首版仅支持单默认 webhook 配置

## 调研结论

### 1. SDK 选型

飞书官方文档推荐使用官方 Go SDK `github.com/larksuite/oapi-sdk-go/v3` 处理服务端 API、事件订阅与卡片回调。基于当前仓库结构，最佳拆分方式是：

1. **自建应用通道使用官方 Go SDK**
   - 适合处理 tenant access token、消息发送、事件订阅、卡片回调
   - 能减少回调校验和 API request/response 拼装的手写工作量

2. **群机器人 webhook 使用标准库轻量实现**
   - 群机器人 webhook 本身是一个单独的 HTTP 协议，与自建应用 app access token/tenant access token 生命周期无关
   - 即使引入 SDK，也仍需单独维护 webhook URL、签名、卡片消息映射
   - 因此机器人 webhook 更适合用标准库直接实现

### 2. 最终建议

采用“**官方 SDK + 仓内轻量适配层**”方案：

- `feishu_app`：使用官方 SDK
- `feishu_bot`：使用标准库实现 webhook client
- Gateway 统一层负责抽象 `ChannelRequest` / `ChannelResponse`

原因：

- 符合用户要求，优先使用官方推荐 SDK
- 自建应用与机器人通道模型不同，共享协议层价值有限，强行统一只会增加耦合
- 当前仓库已经有 `wecom` 适配器模式，最适合新增一套 `feishu` 子目录并保持与其他适配器解耦

## 总体架构

新增 `internal/gateway/feishu/` 目录，内部拆分为以下组件：

- `config.go`：Feishu app/bot 子配置与默认值
- `app_adapter.go`：自建应用适配器，实现 `gateway.RichAdapter`
- `app_handler.go`：事件订阅与卡片回调 HTTP handler
- `app_client.go`：基于官方 SDK 的发送封装
- `app_mapper.go`：事件/消息向统一消息模型的映射
- `bot_adapter.go`：群机器人适配器，实现 `gateway.RichAdapter`
- `bot_client.go`：机器人 webhook 发送与签名
- `bot_payload.go`：机器人文本/卡片 payload 构造

同时对通用 Gateway 层做小幅增强：

- `internal/gateway/types.go`：新增卡片字段与更通用的原始内容承载
- `internal/gateway/router.go`：新增 direct-send 分支
- `internal/gateway/config.go`：从“强制 only wecom”改为“按启用的 adapter 分别校验”
- `cmd/gateway/main.go`：按配置选择性注册 `wecom` / `feishu_app` / `feishu_bot`

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
  - `RawContent json.RawMessage`：保留某些平台原始消息体，避免丢字段

统一消息语义约定：

- `feishu_app` 回调进来的会话消息优先以 `chat_id` 作为 `SessionID`
- `chat_id`、`open_id`、`user_id`、`message_id`、`event_id`、`receive_id_type` 放入 `Metadata`
- 卡片交互回调统一映射为 `MessageTypeEvent`
- 卡片回传更新优先通过 `Card` 字段承载，而不是把 JSON 塞进 `Text`

## 路由设计

当前 `SessionRouter.StartStreamListener()` 只能处理“先有入站 route binding，再有结果回推”的链路；这对 Feishu 群机器人不够，因为机器人 webhook 本身并不提供同构的入站会话回路。

本次新增两个出站模式：

### 1. Route-bound reply（保留现有模式）

适用于：

- `wecom`
- `feishu_app`

行为：

1. `HandleIncoming()` 入队前记录 `task_id -> route binding`
2. `task_results` 回来后查 binding 得到 `channel_name/session_id`
3. 若输出是 legacy `{"result":"..."}`，按文本回推
4. 若输出是 `{"message":{...}}`，按富消息结构回推

### 2. Direct-send reply（新增模式）

适用于：

- `feishu_bot`
- 未来的任意通知型通道

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
    "channel_name": "feishu_bot",
    "session_id": "",
    "message_type": "text",
    "text": "任务执行完成",
    "metadata": {
      "receive_id_type": "chat_id"
    }
  }
}
```

关键约定：

- `message.channel_name` 必填，用于定位 adapter
- direct-send 不要求存在 `task_id -> route binding`
- `session_id` 对通知型 webhook 可为空；是否需要目标地址由 adapter 自行从配置或 `metadata` 决定
- 如果同时存在 route binding 且 `message.channel_name` 也存在，优先使用显式 `message.channel_name`

## Feishu 自建应用设计

### 入站：事件订阅

接入两个 HTTP path：

- `FEISHU_APP_EVENT_CALLBACK_PATH`：事件订阅
- `FEISHU_APP_CARD_CALLBACK_PATH`：卡片交互回调

事件处理策略：

1. 使用官方 SDK 完成请求验签、解密、challenge 响应
2. 从回调体中提取：
   - 文本消息
   - 图片消息
   - 文件消息
   - 机器人/会话上下文
   - 卡片交互事件
3. 映射为 `gateway.ChannelRequest`
4. 投递到 `SessionRouter.HandleIncoming()`
5. HTTP handler 同步返回平台要求的成功响应；后续消息通过异步出站发送

### 会话标识规则

- 首选 `chat_id` 作为 `SessionID`
- 若事件缺少 `chat_id`，回退到 `open_id` 或其他稳定 sender 标识
- 出站优先以 `chat_id` 作为 `receive_id`
- `receive_id_type` 首版固定优先 `chat_id`，并允许在 `Metadata` 中覆盖

### 出站：消息发送

`feishu_app` 支持：

- 文本消息
- Markdown / 富文本消息
- 图片消息
- 文件消息
- 交互卡片消息

发送策略：

1. 根据 `ChannelResponse` 判断消息类型
2. 构造官方 SDK request
3. 根据 `Metadata.receive_id_type` 或默认值设置 `chat_id`
4. 调用 SDK 发送消息

素材处理策略：

- 如果 `MediaItem.MediaID` 已经存在，直接发送
- 如果只有 `Path` 或 `DataBase64`，先上传文件/图片再发送
- 首版不做媒体缓存，按请求即传即发

### 卡片交互回调

卡片回调会被转为统一事件消息：

- `MessageType = event`
- `Text = "card_action"`
- `Card` 中保留完整交互体的关键信息
- `Metadata` 中附带 `action_value`、`open_message_id`、`open_id`、`tenant_key` 等字段

这样 orchestrator 可以把它当作普通入站任务处理，而不需要单独引入新的协议层。

## Feishu 群机器人设计

### 定位

`feishu_bot` 是一个通知型 adapter，不承诺接收用户消息，也不依赖 route binding。它的主要职责是把 Agentic-Core 的结果、告警、摘要、审批提醒以文本或卡片形式发进飞书群。

### 配置

首版仅支持一个默认 webhook：

- `FEISHU_BOT_ENABLED`
- `FEISHU_BOT_WEBHOOK_URL`
- `FEISHU_BOT_SECRET`

### 发送能力

支持：

- 文本
- Markdown 风格富文本（按 webhook 支持的消息结构映射）
- 交互卡片

不做：

- 机器人入站事件
- 多 webhook 路由表

### 签名

如果配置了 `FEISHU_BOT_SECRET`：

- 按官方 webhook 规则生成签名
- 将时间戳和签名写入请求体

如果未配置 secret：

- 允许发送无签名 webhook

## 配置设计

在 `internal/gateway/config.go` 中新增 Feishu 配置，并改造校验逻辑为“按通道启用校验”。

新增配置建议：

- 通用：
  - `GATEWAY_HTTP_PORT`
  - `GATEWAY_REDIS_ADDR`
- WeCom：
  - 保持现有 `WECOM_*`
- Feishu App：
  - `FEISHU_APP_ENABLED`
  - `FEISHU_APP_ID`
  - `FEISHU_APP_SECRET`
  - `FEISHU_APP_VERIFICATION_TOKEN`
  - `FEISHU_APP_ENCRYPT_KEY`
  - `FEISHU_APP_EVENT_CALLBACK_PATH`
  - `FEISHU_APP_CARD_CALLBACK_PATH`
  - `FEISHU_APP_API_BASE_URL`
- Feishu Bot：
  - `FEISHU_BOT_ENABLED`
  - `FEISHU_BOT_WEBHOOK_URL`
  - `FEISHU_BOT_SECRET`

校验规则：

1. 至少启用一个 adapter
2. `wecom` 启用时只校验 `wecom` 必填项
3. `feishu_app` 启用时只校验 app 必填项
4. `feishu_bot` 启用时只校验 webhook URL

这样不会因为只想运行 Feishu 而被 WeCom 必填项阻塞。

## 失败处理与可观测性

### 入站失败

- 回调验签/解密失败：返回 `401/400`
- 回调结构无法识别：返回 `400`
- 入队失败：返回 `502`

### 出站失败

- SDK / webhook 调用失败：返回错误，由 `router` 当前逻辑吞掉但记录日志
- 媒体上传失败：中止当前消息发送
- 配置缺失：adapter 初始化失败，网关启动阶段直接报错

### 日志

新增日志应记录：

- adapter 名称
- callback 类型
- session/chat 标识
- message id / event id
- 发送目标类型
- 平台返回错误码与 request id

但不记录密钥、token、webhook secret、原始敏感内容。

## 测试策略

最小完备测试集合如下：

1. `internal/gateway/config_test.go`
   - Feishu app/bot env + flag 解析
   - 按启用校验，不误伤未启用 adapter

2. `internal/gateway/router_test.go`
   - 保持现有 route-bound reply
   - 新增 direct-send reply
   - direct-send 与 route binding 共存时优先级明确

3. `internal/gateway/feishu/app_handler_test.go`
   - challenge 响应
   - 文本消息回调转统一消息
   - 图片/文件消息回调转统一消息
   - 卡片交互回调转事件消息

4. `internal/gateway/feishu/app_client_test.go`
   - 文本发送 request 映射
   - 卡片消息发送 request 映射
   - 先上传媒体再发送的流程

5. `internal/gateway/feishu/bot_client_test.go`
   - webhook 文本发送 payload
   - 卡片 payload
   - secret 签名

6. `cmd/gateway` 编译/装配验证
   - 仅 Feishu 启用时可成功初始化
   - WeCom + Feishu 共存时 handler 都能注册

## 并行协作约束

当前另一个 Codex 正在开发 `gateway im` 中的企业微信接入，因此本次实现必须避免破坏其工作：

- 不重写 `internal/gateway/wecom/*`
- 对共享文件只做最小必要改动
- 不改变现有 `wecom` 对外行为
- 所有 Feishu 逻辑都放入新增 `internal/gateway/feishu/*`
- 通用增强仅限 `types.go`、`router.go`、`config.go`、`cmd/gateway/main.go`

## 最终建议

按以下顺序实施：

1. 先做通用模型和 router direct-send 增强
2. 再做 Feishu bot webhook，快速打通通知型发送链路
3. 最后做 Feishu app 事件订阅 + API 回发 + 卡片回调

这样既能尽快获得可用结果，也能降低对现有会话型 adapter 的改动风险。
