# Gateway 企业微信联调手册

本文档说明如何启动 `cmd/gateway`，并联调以下三条链路：

- 企业微信自建应用回调：`/callbacks/wecom`
- 通用 JSON 入站：`/v1/channels/incoming`
- 企业微信群机器人 `webhook` 出站：`wecom_robot`

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
- `WECOM_*`：企业微信自建应用配置
- `WECOM_MEDIA_RETENTION_DAYS`：可选。`0` 表示不清理；`>0` 时后台定时清理 `WECOM_MEDIA_DIR` 中超过保留天数的媒体文件
- `WECOM_ROBOT_WEBHOOK_URL`：企业微信群机器人 webhook 地址

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

## 2. 企业微信自建应用回调

### 2.1 回调地址

将企业微信后台回调 URL 指向：

```text
https://<your-domain>/callbacks/wecom
```

对应本地配置：

- `WECOM_TOKEN`
- `WECOM_ENCODING_AES_KEY`
- `WECOM_CALLBACK_PATH`

### 2.2 行为说明

- `GET /callbacks/wecom`：企业微信校验 URL
- `POST /callbacks/wecom`：接收企业微信 XML 回调，统一转为 `gateway.ChannelRequest`
- 图片、语音、视频、文件会自动下载到 `WECOM_MEDIA_DIR`
- 若 `WECOM_MEDIA_RETENTION_DAYS > 0`，Gateway 启动时会立即清理一次过期媒体，随后后台周期性清理

## 3. 通用 JSON 入站

### 3.1 无签名模式

当 `GATEWAY_INGRESS_SECRET` 为空时，可以直接发送：

```bash
curl -sS -X POST http://127.0.0.1:8081/v1/channels/incoming \
  -H 'Content-Type: application/json' \
  -d '{
    "session_id":"web-demo-1",
    "channel_name":"web",
    "message_type":"text",
    "format":"plain_text",
    "text":"hello gateway"
  }'
```

### 3.2 签名模式

当 `GATEWAY_INGRESS_SECRET` 不为空时，请求必须带：

- `X-Agentic-Signature`
- `X-Agentic-Timestamp`
- `X-Agentic-Nonce`

签名算法与审批 webhook 一致：

```text
hex(hmac_sha256(secret, "<timestamp>.<nonce>.<raw_body>"))
```

示例：

```bash
BODY='{"session_id":"secure-robot-1","channel_name":"wecom_robot","message_type":"markdown","format":"markdown","text":"**deploy ok**","metadata":{"wecom_robot_webhook_url":"https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=robot-key"}}'
TS=$(date +%s)
NONCE="nonce-001"
SIG=$(printf "%s.%s.%s" "$TS" "$NONCE" "$BODY" | openssl dgst -sha256 -hmac "$GATEWAY_INGRESS_SECRET" -binary | xxd -p -c 256)

curl -sS -X POST http://127.0.0.1:8081/v1/channels/incoming \
  -H 'Content-Type: application/json' \
  -H "X-Agentic-Timestamp: $TS" \
  -H "X-Agentic-Nonce: $NONCE" \
  -H "X-Agentic-Signature: $SIG" \
  -d "$BODY"
```

期望返回：

```json
{"status":"accepted","task_id":"session_..."}
```

## 4. 企业微信群机器人 webhook 出站

### 4.1 通过统一入口触发机器人消息

`wecom_robot` 是出站通道，推荐通过 `/v1/channels/incoming` 触发。

Markdown 示例：

```bash
curl -sS -X POST http://127.0.0.1:8081/v1/channels/incoming \
  -H 'Content-Type: application/json' \
  -d '{
    "session_id":"robot-demo-1",
    "channel_name":"wecom_robot",
    "message_type":"markdown",
    "format":"markdown",
    "text":"**deploy success**",
    "metadata":{
      "wecom_robot_webhook_url":"https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=robot-key"
    }
  }'
```

图片示例：

```bash
IMAGE_BASE64=$(printf "fake-image" | base64)

curl -sS -X POST http://127.0.0.1:8081/v1/channels/incoming \
  -H 'Content-Type: application/json' \
  -d "{
    \"session_id\":\"robot-image-1\",
    \"channel_name\":\"wecom_robot\",
    \"message_type\":\"image\",
    \"media\":[
      {
        \"kind\":\"image\",
        \"file_name\":\"demo.png\",
        \"data_base64\":\"$IMAGE_BASE64\"
      }
    ],
    \"metadata\":{
      \"wecom_robot_webhook_url\":\"https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=robot-key\"
    }
  }"
```

文件示例：

```bash
FILE_BASE64=$(printf "hello report" | base64)

curl -sS -X POST http://127.0.0.1:8081/v1/channels/incoming \
  -H 'Content-Type: application/json' \
  -d "{
    \"session_id\":\"robot-file-1\",
    \"channel_name\":\"wecom_robot\",
    \"message_type\":\"file\",
    \"media\":[
      {
        \"kind\":\"file\",
        \"file_name\":\"report.txt\",
        \"data_base64\":\"$FILE_BASE64\"
      }
    ],
    \"metadata\":{
      \"wecom_robot_webhook_url\":\"https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=robot-key\"
    }
  }"
```

## 5. 常见问题

### 5.1 为什么 webhook 机器人不能像自建应用一样接收入站消息？

因为企业微信群机器人 `webhook` 协议本身定位是出站通知，不是自建应用消息回调协议。

### 5.2 为什么统一入站推荐 `channel_name=wecom_robot`？

这样可以继续复用 Gateway 的统一路由和出站适配逻辑，而不需要业务侧直接调用企业微信群机器人协议。

### 5.3 媒体文件存在哪里？

企业微信自建应用回调里的媒体会下载到 `WECOM_MEDIA_DIR` 指定目录。

### 5.4 媒体文件会自动清理吗？

会。若设置 `WECOM_MEDIA_RETENTION_DAYS` 为正整数，Gateway 会仅清理 `WECOM_MEDIA_DIR` 顶层目录下超过保留天数的文件；子目录不会被递归删除。
