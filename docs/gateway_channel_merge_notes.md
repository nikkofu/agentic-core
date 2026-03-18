# Gateway 多渠道整合冲突记录（WeCom 先行版）

更新时间：2026-03-18

本文档用于记录企业微信版本在后续与 `feishu`、`dingtalk` 分支整合时，最可能发生冲突的文件与建议处理原则，供后续 Codex 继续处理异常。

## 高概率冲突文件

- `internal/gateway/types.go`
  - 原因：统一消息结构 `StandardMessage`、消息类型枚举、媒体结构体会被多渠道共同扩展。
  - 处理原则：保持字段语义中立，不为单一渠道引入专属顶层字段；渠道差异继续放入 `metadata`。

- `internal/gateway/router.go`
  - 原因：多渠道都会注册适配器、依赖统一回推链路、可能继续调整 route binding。
  - 处理原则：优先保留当前“统一入站 + task_id 绑定 + RichAdapter 回推”的主链路，不回退到渠道分叉逻辑。

- `internal/gateway/config.go`
  - 原因：Feishu、DingTalk 都会新增各自配置段与环境变量。
  - 处理原则：继续按 channel 分段集中管理；不要把渠道配置散落回 `cmd/*`。

- `cmd/gateway/main.go`
  - 原因：多渠道都会在这里注册 HTTP callback、adapter、runtime cleanup 任务。
  - 处理原则：保留集中启动入口；新增渠道时按独立初始化块追加，不要交叉耦合 WeCom/Feishu/DingTalk 初始化。

- `cmd/gateway/.env.example`
  - 原因：多个渠道都会补充 env 示例。
  - 处理原则：按渠道分组排序，避免重复或名称冲突。

- `internal/gateway/ingress_handler.go`
  - 原因：统一签名入站是机器人/webhook 渠道复用点，其他渠道可能补测试或 metadata 约定。
  - 处理原则：统一入口协议保持不变，避免为单渠道改请求结构。

## WeCom 已落地的边界约束

- `wecom` 自建应用：
  - 支持回调接收、统一解析、异步回推
  - 支持文字、Markdown、图片、音频、视频、文件、news
  - 入站支持 text/image/voice/video/file/link/location/event

- `wecom_robot`：
  - 仅支持出站
  - 当前支持 `text`、`markdown`、`image`、`file`、`news`
  - 不应被后续渠道整合误改成伪入站 source

## 建议整合顺序

1. 先合并 `internal/gateway/types.go`
2. 再合并 `internal/gateway/config.go`
3. 再合并 `internal/gateway/router.go`
4. 最后处理 `cmd/gateway/main.go` 和 `cmd/gateway/.env.example`

这样可以先统一模型，再统一装配，降低 callback 注册冲突。

## 推荐检查清单

- `go test ./internal/gateway ./internal/gateway/wecom ./internal/gateway/wecomrobot ./cmd/gateway -v`
- 确认 `/v1/channels/incoming` 协议未变化
- 确认 `StandardMessage` 没被分叉成多个渠道私有版本
- 确认 `.env.example` 中各渠道变量命名不冲突
- 确认 `wecom_robot` 仍保持出站-only
