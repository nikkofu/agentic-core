# Gateway 钉钉联调手册

本文档说明如何启动 `cmd/gateway`，并联调以下四条 DingTalk 链路：

- 钉钉企业应用事件回调：`/callbacks/dingtalk/events`
- 钉钉企业应用卡片回调：`/callbacks/dingtalk/cards`
- 钉钉企业应用出站：`dingtalk_app`
- 钉钉群机器人 `webhook` 出站：`dingtalk_robot`

当前实现以中国站 DingTalk 为准，企业应用入站使用 HTTP 回调模式，不启用 Stream Mode。

## 1. 启动前准备

### 1.1 环境变量

可基于 `cmd/gateway/.env.example` 复制配置：

```bash
cp cmd/gateway/.env.example /tmp/gateway.env
```

核心参数：

- `GATEWAY_HTTP_PORT`：Gateway 监听地址，默认 `:8081`
- `GATEWAY_REDIS_ADDR`：Redis 地址
- `GATEWAY_INGRESS_SECRET`：可选。若设置，则 `POST /v1/channels/incoming` 必须带签名头
- `DINGTALK_APP_*`：钉钉企业应用配置
- `DINGTALK_ROBOT_*`：钉钉群机器人 webhook 配置

其中：

- `DINGTALK_APP_EVENT_CALLBACK_PATH` 默认 `"/callbacks/dingtalk/events"`
- `DINGTALK_APP_CARD_CALLBACK_PATH` 默认 `"/callbacks/dingtalk/cards"`
- `DINGTALK_APP_API_BASE_URL` 可留空，留空时使用 DingTalk SDK 默认 `api.dingtalk.com`
- `DINGTALK_APP_OAPI_BASE_URL` 可留空，留空时使用旧开放平台默认 `oapi.dingtalk.com`
- `DINGTALK_APP_MEDIA_DIR` 当前为预留字段，首版不会在入站回调时自动把媒体落盘，可留空
- `DINGTALK_APP_CARD_TEMPLATE_ID` 可选，用于交互卡片的默认模板 ID
- `DINGTALK_APP_CARD_CALLBACK_ROUTE_KEY` 可选，用于交互卡片的默认回调路由 key

### 1.2 本地启动

```bash
set -a
source /tmp/gateway.env
set +a
go run ./cmd/gateway
```

健康检查：

```bash
curl -sS http://127.0.0.1:8081/healthz
```

期望返回：

```text
ok
```

## 2. 钉钉企业应用回调

### 2.1 回调地址

将钉钉开放平台中的事件订阅地址配置为：

```text
https://<your-domain>/callbacks/dingtalk/events
```

将卡片回调地址配置为：

```text
https://<your-domain>/callbacks/dingtalk/cards
```

对应本地配置：

- `DINGTALK_APP_CLIENT_ID`
- `DINGTALK_APP_CLIENT_SECRET`
- `DINGTALK_APP_TOKEN`
- `DINGTALK_APP_AES_KEY`
- `DINGTALK_APP_EVENT_CALLBACK_PATH`
- `DINGTALK_APP_CARD_CALLBACK_PATH`

### 2.2 行为说明

- `POST /callbacks/dingtalk/events`：处理事件 challenge、消息事件回调
- `POST /callbacks/dingtalk/cards`：处理卡片 challenge、卡片动作回调
- 当回调体为 `{"encrypt":"..."}` 时，Gateway 会按 `signature/timestamp/nonce` 校验签名、解密消息，并在成功后回加密 `success`
- 当前已支持入站消息类型：
  - 文本
  - 图片
  - 语音
  - 视频
  - 文件
  - 事件
  - 卡片动作事件
- Gateway 会将回调统一映射为 `gateway.ChannelRequest` 后投递给 orchestrator

### 2.3 返回语义

- `200`：challenge 校验成功，或事件已成功入队
- `401`：加密回调签名不合法
- `400`：请求体非法 / 无法解析
- `502`：Gateway 投递内部任务失败

### 2.4 本地验证建议

明文 challenge 验证可直接使用 `curl`：

```bash
curl -sS -X POST http://127.0.0.1:8081/callbacks/dingtalk/events \
  -H 'Content-Type: application/json' \
  -d '{"challenge":"challenge-token"}'
```

期望返回：

```json
{"challenge":"challenge-token"}
```

加密回调建议直接运行已有单测做本地校验，因为构造 `encrypt + signature + timestamp + nonce` 需要按钉钉协议加密并签名：

```bash
go test ./internal/gateway/dingtalk -run 'TestAppHandlerDecryptsEncryptedCallbackAndReturnsEncryptedAck|TestAppHandlerRejectsEncryptedCallbackWithInvalidSignature' -v
```

若要联调真实平台，则把 DingTalk 开放平台事件订阅地址直接指向暴露出来的 `https://<your-domain>/callbacks/dingtalk/events` 和 `https://<your-domain>/callbacks/dingtalk/cards` 即可。

## 3. 钉钉企业应用出站

### 3.1 出站能力

`dingtalk_app` 当前支持：

- 文本
- Markdown
- 图片
- 语音
- 视频
- 文件
- 交互卡片
- action-card / link / oa 风格卡片透传

实现策略分两层：

- 优先使用 `conversationId + chatbotUserId` 回发到原始会话
- 若当前消息没有会话目标，则回退到工作通知发送

其中 access token 获取与交互卡片发送走官方 DingTalk SDK；普通消息发送与媒体上传走旧开放平台接口，以覆盖当前 SDK 未封装的企业应用消息能力。

### 3.2 会话回发

当消息源来自 `dingtalk_app` 入站回调时，Gateway 已自动保留以下 metadata：

- `conversationId`
- `chatbotUserId`
- `senderStaffId`
- `msgId`

因此 orchestrator 只要回推：

```json
{
  "message": {
    "channel_name": "dingtalk_app",
    "message_type": "text",
    "text": "hello from orchestrator"
  }
}
```

Gateway 会优先回到原钉钉会话。

Markdown 示例：

```json
{
  "message": {
    "channel_name": "dingtalk_app",
    "message_type": "markdown",
    "format": "markdown",
    "text": "## deploy ok",
    "metadata": {
      "title": "Deploy Status"
    }
  }
}
```

### 3.3 工作通知回退

若当前消息不是从既有 `dingtalk_app` route binding 派生，而是主动通知，则可通过下列字段指定接收人：

- `receiver_id`
- `metadata.dingtalk_userid_list`
- `metadata.dingtalk_dept_id_list`
- `metadata.dingtalk_to_all_user`

示例：

```json
{
  "message": {
    "channel_name": "dingtalk_app",
    "receiver_id": "manager123",
    "message_type": "text",
    "text": "nightly deploy finished"
  }
}
```

若需要一次发给多人：

```json
{
  "message": {
    "channel_name": "dingtalk_app",
    "message_type": "markdown",
    "format": "markdown",
    "text": "## rollout done",
    "metadata": {
      "title": "Release",
      "dingtalk_userid_list": ["user-a", "user-b"]
    }
  }
}
```

### 3.4 交互卡片

当 `message.card.cardTemplateId` 存在时，Gateway 会走 DingTalk 交互卡片接口。

支持字段：

- `card.cardTemplateId`
- `card.cardParamMap`
- `card.cardMediaIdParamMap`
- `card.outTrackId`
- `card.callbackRouteKey`
- `card.conversationType`
- `card.supportForward`

其中 `card.cardTemplateId` 与 `card.callbackRouteKey` 也可分别由：

- `DINGTALK_APP_CARD_TEMPLATE_ID`
- `DINGTALK_APP_CARD_CALLBACK_ROUTE_KEY`

提供默认值。

示例：

```json
{
  "message": {
    "channel_name": "dingtalk_app",
    "session_id": "cidxxxxxx",
    "card": {
      "cardTemplateId": "tpl_123",
      "conversationType": 2,
      "cardParamMap": {
        "title": "Deploy OK",
        "status": "done"
      },
      "callbackRouteKey": "gateway-review"
    },
    "metadata": {
      "chatbotUserId": "$:LWCP_v1:$demo-bot"
    }
  }
}
```

### 3.5 媒体发送

`dingtalk_app` 发送图片、语音、视频、文件时：

- 若 `media[].media_id` 已存在，Gateway 会直接复用
- 若不存在，Gateway 会先调用 `media/upload` 上传，再发送普通消息

图片示例：

```json
{
  "message": {
    "channel_name": "dingtalk_app",
    "message_type": "image",
    "session_id": "cidxxxxxx",
    "media": [
      {
        "kind": "image",
        "file_name": "demo.png",
        "data_base64": "<base64>"
      }
    ],
    "metadata": {
      "chatbotUserId": "$:LWCP_v1:$demo-bot"
    }
  }
}
```

## 4. 钉钉群机器人 webhook 出站

### 4.1 通过统一入口触发

`dingtalk_robot` 是纯出站通道，推荐通过 `/v1/channels/incoming` 触发。

文本示例：

```bash
curl -sS -X POST http://127.0.0.1:8081/v1/channels/incoming \
  -H 'Content-Type: application/json' \
  -d '{
    "session_id":"ding-robot-text-1",
    "channel_name":"dingtalk_robot",
    "message_type":"text",
    "text":"deploy success"
  }'
```

Markdown 示例：

```bash
curl -sS -X POST http://127.0.0.1:8081/v1/channels/incoming \
  -H 'Content-Type: application/json' \
  -d '{
    "session_id":"ding-robot-markdown-1",
    "channel_name":"dingtalk_robot",
    "message_type":"markdown",
    "format":"markdown",
    "text":"## deploy success",
    "metadata":{
      "title":"Deploy Status"
    }
  }'
```

action-card 示例：

```bash
curl -sS -X POST http://127.0.0.1:8081/v1/channels/incoming \
  -H 'Content-Type: application/json' \
  -d '{
    "session_id":"ding-robot-card-1",
    "channel_name":"dingtalk_robot",
    "card":{
      "msgtype":"actionCard",
      "actionCard":{
        "title":"Alert",
        "text":"### investigate",
        "singleTitle":"Open",
        "singleURL":"https://example.com"
      }
    }
  }'
```

### 4.2 webhook 覆盖规则

`dingtalk_robot` 的 webhook 地址解析优先级为：

1. `message.metadata.dingtalk_robot_webhook_url`
2. `DINGTALK_ROBOT_WEBHOOK_URL`

签名 secret 解析优先级为：

1. `message.metadata.dingtalk_robot_secret`
2. `DINGTALK_ROBOT_SECRET`

若配置了 secret，Gateway 会自动追加 `timestamp` 和 `sign`。

### 4.3 当前边界

当前 `dingtalk_robot` webhook 路径支持：

- 文本
- Markdown
- action-card 透传

以下类型目前不会静默降级，而是直接报错：

- 图片
- 语音
- 视频
- 文件
- 其他未识别 `message_type`

## 5. Direct-Send 回推

Gateway 支持 orchestrator 在 `task_results` 中显式指定 `channel_name`，因此 `dingtalk_robot` 这类纯通知通道不要求先存在入站 route binding。

示例：

```json
{
  "message": {
    "channel_name": "dingtalk_robot",
    "message_type": "text",
    "text": "nightly deploy finished"
  }
}
```

若显式给出 `channel_name=dingtalk_app`，且 metadata 同时带上 `conversationId` / `chatbotUserId` 或工作通知目标字段，也可以直接把消息路由到钉钉企业应用。

## 6. 常见问题

### 6.1 为什么 `dingtalk_app` 同时用了 SDK 和旧开放平台接口？

因为当前官方 Go SDK 已覆盖 access token 与交互卡片接口，但企业应用普通消息发送与媒体上传仍更适合走旧开放平台接口；混合实现能在不破坏现有 gateway 模型的前提下覆盖更多能力。

### 6.2 为什么 `dingtalk_robot` 和 `dingtalk_app` 要拆开？

两者协议边界不同：

- `dingtalk_app`：支持双向事件回调、媒体、工作通知、交互卡片
- `dingtalk_robot`：适合群内播报与告警 webhook

### 6.3 钉钉企业应用回发为什么优先用 `conversationId`？

因为入站回调已经携带 `conversationId` 与 `chatbotUserId`，Gateway 可以直接回到原会话，不需要额外业务层再维护一份钉钉会话映射。

### 6.4 如果没有原始会话，还能主动推送吗？

可以。给 `receiver_id` 或 `metadata.dingtalk_userid_list`，Gateway 会回退到工作通知发送。
