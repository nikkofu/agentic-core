# Gateway 飞书联调手册

本文档说明如何启动 `cmd/gateway`，并联调以下三条 Feishu 链路：

- 飞书自建应用事件回调：`/callbacks/feishu/events`
- 飞书卡片回调：`/callbacks/feishu/cards`
- 飞书群机器人 `webhook` 出站：`feishu_bot`

当前手册以中国站 Feishu 为准，不覆盖国际站 Lark 的单独配置差异。

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
- `FEISHU_APP_*`：飞书自建应用配置
- `FEISHU_BOT_*`：飞书群机器人配置

其中：

- `FEISHU_APP_EVENT_CALLBACK_PATH` 默认 `"/callbacks/feishu/events"`
- `FEISHU_APP_CARD_CALLBACK_PATH` 默认 `"/callbacks/feishu/cards"`
- `FEISHU_APP_API_BASE_URL` 可留空，留空时使用 SDK 默认开放平台地址

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

## 2. 飞书自建应用回调

### 2.1 回调地址

将飞书开放平台中的事件订阅地址配置为：

```text
https://<your-domain>/callbacks/feishu/events
```

将卡片回调地址配置为：

```text
https://<your-domain>/callbacks/feishu/cards
```

对应本地配置：

- `FEISHU_APP_ID`
- `FEISHU_APP_SECRET`
- `FEISHU_APP_VERIFICATION_TOKEN`
- `FEISHU_APP_ENCRYPT_KEY`
- `FEISHU_APP_EVENT_CALLBACK_PATH`
- `FEISHU_APP_CARD_CALLBACK_PATH`

### 2.2 行为说明

- `POST /callbacks/feishu/events`：处理事件订阅 challenge、消息事件回调
- `POST /callbacks/feishu/cards`：处理卡片 challenge、卡片按钮/表单回调
- 当前已支持入站消息类型：
  - 文本
  - 图片
  - 文件
  - 卡片动作事件
- Gateway 会将回调统一映射为 `gateway.ChannelRequest` 后投递给 orchestrator

### 2.3 返回语义

- `200`：challenge 校验成功，或事件已成功入队
- `400`：请求体非法 / 无法解析
- `401`：签名或 token 不合法
- `502`：Gateway 投递内部任务失败

## 3. 飞书群机器人 webhook 出站

### 3.1 通过统一入口触发

`feishu_bot` 是出站通道，推荐通过 `/v1/channels/incoming` 触发。

文本示例：

```bash
curl -sS -X POST http://127.0.0.1:8081/v1/channels/incoming \
  -H 'Content-Type: application/json' \
  -d '{
    "session_id":"feishu-bot-text-1",
    "channel_name":"feishu_bot",
    "message_type":"text",
    "text":"deploy success"
  }'
```

卡片示例：

```bash
curl -sS -X POST http://127.0.0.1:8081/v1/channels/incoming \
  -H 'Content-Type: application/json' \
  -d '{
    "session_id":"feishu-bot-card-1",
    "channel_name":"feishu_bot",
    "card":{
      "config":{"wide_screen_mode":true},
      "elements":[
        {"tag":"div","text":{"tag":"lark_md","content":"**deploy success**"}}
      ]
    }
  }'
```

说明：

- 若环境里已设置 `FEISHU_BOT_WEBHOOK_URL`，可以直接使用
- 若配置了 `FEISHU_BOT_SECRET`，Gateway 会自动附加飞书要求的 `timestamp` 与 `sign`

## 4. 飞书自建应用出站

### 4.1 统一回推消息类型

`feishu_app` 出站当前支持：

- 文本
- `post`
- 交互式卡片
- 图片
- 文件

默认情况下，`SessionID` 会被当作 `chat_id` 使用；也可以通过 `metadata.receive_id_type` 指定接收 ID 类型。

示例：

```json
{
  "message": {
    "channel_name": "feishu_app",
    "session_id": "oc_a6f1...",
    "message_type": "text",
    "text": "hello from orchestrator"
  }
}
```

若要指定接收 ID 类型：

```json
{
  "message": {
    "channel_name": "feishu_app",
    "receiver_id": "ou_xxx",
    "message_type": "text",
    "text": "hello user",
    "metadata": {
      "receive_id_type": "open_id"
    }
  }
}
```

## 5. Direct-Send 回推

Gateway 现在支持 orchestrator 在 `task_results` 中显式指定目标通道，适用于 `feishu_bot` 这类纯通知通道，不要求先存在入站 route binding。

示例输出：

```json
{
  "message": {
    "channel_name": "feishu_bot",
    "message_type": "text",
    "text": "nightly deploy finished",
    "metadata": {
      "tenant": "demo"
    }
  }
}
```

说明：

- 若 `result.TaskID` 对应已有 route binding，Gateway 会把 binding metadata 与显式消息 metadata 合并
- 若 `message.channel_name` 已显式给出，则显式通道优先

## 6. 常见问题

### 6.1 为什么 `feishu_bot` 只能出站？

因为飞书群机器人 webhook 协议本身定位是出站通知协议，不是事件回调协议。

### 6.2 为什么 `feishu_app` 和 `feishu_bot` 要拆成两个通道？

两者鉴权模型、协议能力和消息边界不同：

- `feishu_app`：适合双向交互、事件回调、媒体发送、卡片回调
- `feishu_bot`：适合告警、通知、群内播报

### 6.3 飞书图片和文件出站怎么处理？

若 `media[].media_id` 已存在，Gateway 会直接复用；否则会先上传媒体，再发送消息。

### 6.4 飞书 challenge 回调为什么必须带 `Verification Token`？

因为 Gateway 会校验 challenge 请求中的 token，确保它和本地 `FEISHU_APP_VERIFICATION_TOKEN` 一致。
