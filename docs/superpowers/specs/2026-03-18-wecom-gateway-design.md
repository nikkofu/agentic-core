# 企业微信 Gateway 接入设计

## 目标

在现有 `internal/gateway` 的统一收发模型之上，补齐企业微信（WeCom / 企业微信自建应用）接入能力，支持：

- 接收：文字、图片、音频（企业微信 `voice`）、视频、文件、链接/位置/事件等统一入站解析
- 回复：文字、Markdown、图片、音频、视频、文件，以及统一的富媒体消息载体
- 路由：严格走现有 Gateway → Queue → Orchestrator → `task_results` → Adapter 回传链路
- 配置：集中管理企业微信凭证、回调、加解密、API 地址等参数
- 补充：支持企业微信群机器人 `webhook` 出站通道

## 调研结论

### 1. SDK 选型

当前 Go 生态里没有发现覆盖企业微信消息回调、回调加解密、应用消息发送、媒体上传、统一类型映射且由腾讯官方维护的完整 Go SDK。

候选方案：

1. `github.com/silenceper/wechat`
   - 优点：国内使用面更广，覆盖公众号/小程序/企业微信生态，文档和历史版本较多。
   - 缺点：抽象层较厚，企业微信能力分散；对于本仓库要求的“严格遵守统一 Gateway 接口”而言，仍需大量二次包装。

2. `github.com/xen0n/go-workwx`
   - 优点：更聚焦企业微信，类型定义更清晰，包含回调处理与 Markdown/媒体消息能力。
   - 缺点：不是官方 SDK；若仅为本项目引入，仍要为统一收发结构做适配层，并新增外部依赖。

3. 直接对接企业微信官方 HTTP 协议
   - 优点：最贴合现有 `gateway` 统一接口和当前轻量依赖策略；能力边界清晰；配置、鉴权、媒体上传、回调解密都可按本仓库风格落地。
   - 缺点：需要自行实现回调验签/AES 解密/token 缓存/消息类型映射。

### 2. 最终建议

本次实现优先采用“**官方协议 + 仓内轻量适配层**”方案，不直接引入第三方 SDK。

原因：

- 当前仓库尚未形成稳定的 `gateway/adapter` 分层，先把统一接口收紧比引 SDK 更重要。
- 企业微信接入只需用到回调验证、消息解密、消息发送、临时素材上传四块核心能力，自己实现成本可控。
- 避免增加无官方背书的第三方依赖，同时保留后续切换到 `silenceper/wechat` 或 `go-workwx` 的可能。

### 3. 关于 webhook 机器人的结论

企业微信群机器人 `webhook` 与企业微信自建应用是两条不同协议：

- 自建应用：支持回调 URL 校验、消息回调、应用消息发送、素材上传下载
- 群机器人 `webhook`：本次调研结论是**主要作为出站推送通道**，适合告警/通知类消息，不作为本项目的 IM 会话入站源

因此本次实现对 webhook 机器人采用“**出站适配器**”形态，不给它增加伪回调入口。

## 统一接口设计

新增 `internal/gateway/types.go`，引入 `StandardMessage` 作为统一收发载体，并向后兼容现有 `ChannelRequest`：

- 基础字段：`session_id`、`channel_name`、`message_id`、`sender_id`、`sender_name`
- 语义字段：`message_type`、`format`、`text`
- 媒体字段：`media[]`
- 富媒体字段：`articles[]`
- 扩展字段：`metadata`

同时保留旧的 `Adapter.SendMessage(ctx, sessionID, text)` 纯文本接口，新增可选富消息接口，避免破坏既有测试和 Mock。

## 路由设计

修复当前 `SessionRouter.StartStreamListener()` 里“结果一律回给 `mock` 通道”的硬编码缺陷。

新的路由行为：

1. `HandleIncoming()` 入队前记录 `task_id -> route binding`
2. `task_results` 回来后，优先查绑定关系得到 `channel_name/session_id`
3. 若输出是 legacy `{"result":"..."}`，按文本回推
4. 若输出是 `{"message":{...}}`，按统一富消息结构回推

## 企业微信接入设计

### 通用 JSON 入站

- `POST /v1/channels/incoming`
- 请求体直接使用统一 `gateway.ChannelRequest`
- 目标：为 `wecom_robot`、`web`、后续 Slack/钉钉 等渠道提供统一联调入口
- 返回：`202 Accepted` + `task_id`
- 若配置 `GATEWAY_INGRESS_SECRET`，则请求必须携带 `X-Agentic-Signature`、`X-Agentic-Timestamp`、`X-Agentic-Nonce`

### 入站

- `GET /callbacks/wecom`：完成 URL 校验
- `POST /callbacks/wecom`：
  - 验签
  - 解密回调 XML
  - 解析 text/image/voice/video/file/link/location/event
  - 转为 `gateway.StandardMessage`
  - 投递到 `SessionRouter.HandleIncoming()`
  - 返回 `200 OK` 空体，采用异步发送应用消息回推

### 出站

- 文本：`message/send` + `text`
- Markdown：`message/send` + `markdown`
- 图片/音频/视频/文件：
  - 若已有 `media_id`，直接发送
  - 若给的是 `path` 或 `data_base64`，先走 `media/upload`
- 富媒体：
  - 统一载体先在接口层保留
  - 本次先实现企业微信 `news` 发送，`mpnews` 暂不引入永久素材链路

### 企业微信群机器人 webhook

- 通道名：`wecom_robot`
- 能力边界：出站-only
- 推荐入站方式：若业务侧已有自己的 webhook/告警平台，可先转换成统一 `POST /v1/channels/incoming` 请求再进入本 Gateway
- 本次支持：
  - `text`
  - `markdown`
  - `image`（按 webhook 协议发送 `base64 + md5`）
  - `file`（先 `upload_media`，再按 `media_id` 发送）
  - `news`
- 本次不支持：
  - 音频/视频回推（机器人 webhook 协议不作为本次实现目标）

## 配置设计

新增集中式 Gateway 配置：

- `GATEWAY_HTTP_PORT`
- `GATEWAY_REDIS_ADDR`
- `GATEWAY_INGRESS_SECRET`
- `WECOM_CORP_ID`
- `WECOM_AGENT_ID`
- `WECOM_CORP_SECRET`
- `WECOM_TOKEN`
- `WECOM_ENCODING_AES_KEY`
- `WECOM_CALLBACK_PATH`
- `WECOM_API_BASE_URL`
- `WECOM_MEDIA_DIR`
- `WECOM_MEDIA_RETENTION_DAYS`
- `WECOM_ROBOT_WEBHOOK_URL`

配置解析逻辑统一放到 `internal/gateway/config.go`，`cmd/gateway` 只负责启动。

其中 `WECOM_MEDIA_RETENTION_DAYS` 用于控制回调下载媒体的本地保留策略：`0` 为关闭自动清理，正整数表示保留天数，Gateway 启动后会立即执行一次清理并在后台定时清理过期文件。

## 测试策略

- Router：验证 route binding、legacy 文本回推、富消息回推
- Crypto：验签、加解密、URL 校验
- Callback Handler：覆盖 text/image/voice/video/file 回调转标准消息
- Callback Handler：覆盖 text/image/voice/video/file 回调转标准消息，以及入站媒体自动下载
- Client：token 缓存、Markdown 发送、媒体上传再发送
- Robot Adapter：覆盖 webhook markdown/image/file/news 发送
- Config：环境变量 + flag 解析

## 非目标

- 不在本次范围内引入群聊会话编排
- 不在本次范围内实现永久素材 `mpnews`
- 不改动 orchestrator/subagent 的核心推理协议，仅扩展 `task_results` 的 Gateway 出站解释能力
