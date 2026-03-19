# DingTalk Gateway Integration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add China-region DingTalk gateway support with both enterprise app HTTP callbacks and group robot webhook sending, while preserving existing `wecom` and `feishu` behavior and limiting shared-layer changes to DingTalk-specific configuration and wiring.

**Architecture:** Reuse the already-extended shared `gateway.StandardMessage` and `SessionRouter`, add a new `internal/gateway/dingtalk` package split into `app` and `robot` responsibilities, and wire both adapters into `cmd/gateway`. Use the official DingTalk Go SDK package family `github.com/alibabacloud-go/dingtalk` for app-side OpenAPI calls, keep callback verification/mapping in lightweight in-repo handlers, and implement robot webhook sending with stdlib HTTP/signing helpers.

**Tech Stack:** Go 1.25.x, existing Redis transport, existing gateway queue model, official DingTalk Go SDK modules under `github.com/alibabacloud-go/dingtalk`, stdlib `net/http` / `crypto/hmac` / `encoding/json`, existing `go test`.

---

## File Map

- `go.mod`
  - Add the official DingTalk Go SDK dependency and any required Tea/OpenAPI support packages pulled in by the SDK.
- `go.sum`
  - Record DingTalk SDK and transitive dependency checksums.
- `internal/gateway/config.go`
  - Add top-level `DingTalkAppConfig` and `DingTalkRobotConfig`, env/flag parsing, and enablement-aware validation.
- `internal/gateway/config_test.go`
  - Verify DingTalk app-only and robot-only config parsing and mixed-adapter validation.
- `cmd/gateway/main.go`
  - Conditionally register `dingtalk_app` and `dingtalk_robot` without changing existing `wecom` / `feishu` semantics.
- `cmd/gateway/main_test.go`
  - Cover DingTalk-only mux initialization and callback route registration.
- `cmd/gateway/.env.example`
  - Add all DingTalk runtime configuration examples.
- `internal/gateway/dingtalk/config.go`
  - Hold DingTalk app/robot runtime config defaults, normalization, and validation helpers.
- `internal/gateway/dingtalk/callback_types.go`
  - Define typed callback/request payload envelopes used by inbound handler tests and mappers.
- `internal/gateway/dingtalk/app_support.go`
  - Hold callback signature / decrypt helpers and app-client-facing helper interfaces.
- `internal/gateway/dingtalk/app_mapper.go`
  - Convert DingTalk event callbacks and card action callbacks into `gateway.ChannelRequest`.
- `internal/gateway/dingtalk/app_handler.go`
  - Handle event callback and card callback HTTP requests.
- `internal/gateway/dingtalk/app_adapter.go`
  - Provide `gateway.RichAdapter` behavior for `dingtalk_app`.
- `internal/gateway/dingtalk/app_client.go`
  - Wrap the official DingTalk SDK for token, send, and upload operations.
- `internal/gateway/dingtalk/robot_payload.go`
  - Build webhook payloads from `gateway.ChannelResponse`.
- `internal/gateway/dingtalk/robot_client.go`
  - Send DingTalk robot webhook payloads and compute optional signatures.
- `internal/gateway/dingtalk/robot_adapter.go`
  - Provide `gateway.RichAdapter` behavior for `dingtalk_robot`.
- `internal/gateway/dingtalk/app_handler_test.go`
  - Test challenge handling, callback mapping, and card action mapping.
- `internal/gateway/dingtalk/app_client_test.go`
  - Test app-side send payload mapping and upload-then-send flow with stubs.
- `internal/gateway/dingtalk/robot_client_test.go`
  - Test webhook payloads, metadata overrides, signing, and unsupported media behavior.
- `docs/gateway_dingtalk_runbook.md`
  - Document runtime config, callback URLs, webhook examples, and local test flows.
- `docs/README.md`
  - Add the DingTalk runbook to the documentation index.

### Task 1: Add DingTalk gateway config and bootstrap wiring

**Files:**
- Modify: `internal/gateway/config.go`
- Test: `internal/gateway/config_test.go`
- Modify: `cmd/gateway/main.go`
- Test: `cmd/gateway/main_test.go`
- Modify: `cmd/gateway/.env.example`

- [ ] **Step 1: Write the failing config and mux tests**

```go
func TestParseConfigAllowsDingTalkAppOnlyMode(t *testing.T) {}
func TestParseConfigAllowsDingTalkRobotOnlyMode(t *testing.T) {}
func TestGatewayCanInitializeWithDingTalkOnlyConfig(t *testing.T) {}
func TestGatewayRegistersDingTalkHandlersWhenEnabled(t *testing.T) {}
```

- [ ] **Step 2: Run the focused config and mux tests to verify failure**

Run: `go test ./internal/gateway ./cmd/gateway -run 'TestParseConfigAllowsDingTalkAppOnlyMode|TestParseConfigAllowsDingTalkRobotOnlyMode|TestGatewayCanInitializeWithDingTalkOnlyConfig|TestGatewayRegistersDingTalkHandlersWhenEnabled' -v`

Expected: FAIL because no DingTalk config types, env parsing, or gateway mux registration exist yet.

- [ ] **Step 3: Write the minimal top-level config and wiring implementation**

Add:

```go
type DingTalkAppConfig struct {
    Enabled           bool
    ClientID          string
    ClientSecret      string
    AgentID           int64
    EventCallbackPath string
    CardCallbackPath  string
    APIBaseURL        string
    OAPIBaseURL       string
    Token             string
    AESKey            string
    MediaDir          string
}

type DingTalkRobotConfig struct {
    Enabled    bool
    WebhookURL string
    Secret     string
}
```

Wire these rules:

- At least one adapter across `wecom`, `wecom_robot`, `feishu_app`, `feishu_bot`, `dingtalk_app`, `dingtalk_robot` must be enabled
- Validate only enabled DingTalk adapters
- Register `/callbacks/dingtalk/events` and `/callbacks/dingtalk/cards` only when `dingtalk_app` is enabled
- Register `dingtalk_robot` adapter only when webhook config is present
- Add all `DINGTALK_*` vars to `cmd/gateway/.env.example`

- [ ] **Step 4: Run the focused config and mux tests to verify success**

Run: `go test ./internal/gateway ./cmd/gateway -run 'TestParseConfigAllowsDingTalkAppOnlyMode|TestParseConfigAllowsDingTalkRobotOnlyMode|TestGatewayCanInitializeWithDingTalkOnlyConfig|TestGatewayRegistersDingTalkHandlersWhenEnabled' -v`

Expected: PASS

### Task 2: Implement DingTalk robot webhook sending

**Files:**
- Create: `internal/gateway/dingtalk/config.go`
- Create: `internal/gateway/dingtalk/robot_payload.go`
- Create: `internal/gateway/dingtalk/robot_client.go`
- Create: `internal/gateway/dingtalk/robot_adapter.go`
- Test: `internal/gateway/dingtalk/robot_client_test.go`

- [ ] **Step 1: Write the failing robot tests**

```go
func TestRobotClientBuildsTextPayload(t *testing.T) {}
func TestRobotClientBuildsMarkdownPayload(t *testing.T) {}
func TestRobotClientBuildsCardPayload(t *testing.T) {}
func TestRobotClientUsesMetadataWebhookOverride(t *testing.T) {}
func TestRobotClientRejectsUnsupportedMediaMessages(t *testing.T) {}
func TestRobotAdapterSendUsesWebhookClient(t *testing.T) {}
```

- [ ] **Step 2: Run the focused robot tests to verify failure**

Run: `go test ./internal/gateway/dingtalk -run 'TestRobotClientBuildsTextPayload|TestRobotClientBuildsMarkdownPayload|TestRobotClientBuildsCardPayload|TestRobotClientUsesMetadataWebhookOverride|TestRobotClientRejectsUnsupportedMediaMessages|TestRobotAdapterSendUsesWebhookClient' -v`

Expected: FAIL because no DingTalk robot implementation exists yet.

- [ ] **Step 3: Write the minimal robot implementation**

Implement:

```go
type RobotConfig struct {
    WebhookURL  string
    Secret      string
    HTTPTimeout time.Duration
}

func buildRobotPayload(msg gateway.ChannelResponse) (map[string]any, error) {}
func (c *RobotClient) Send(ctx context.Context, msg gateway.ChannelResponse) error {}
func (a *RobotAdapter) Send(ctx context.Context, msg gateway.ChannelResponse) error {}
```

Mapping rules:

- `MessageTypeText` → DingTalk robot `text`
- `FormatMarkdown` or `MessageTypeMarkdown` → DingTalk robot `markdown`
- `Card != nil` → DingTalk card / action-card-compatible webhook payload
- Resolve `webhook_url` and `secret` from message metadata first, config second
- If the webhook protocol does not support a requested audio / video / file payload directly, return a clear `unsupported dingtalk robot message type` error instead of silently downgrading

- [ ] **Step 4: Run the focused robot tests to verify success**

Run: `go test ./internal/gateway/dingtalk -run 'TestRobotClientBuildsTextPayload|TestRobotClientBuildsMarkdownPayload|TestRobotClientBuildsCardPayload|TestRobotClientUsesMetadataWebhookOverride|TestRobotClientRejectsUnsupportedMediaMessages|TestRobotAdapterSendUsesWebhookClient' -v`

Expected: PASS

### Task 3: Implement DingTalk app inbound callback mapping

**Files:**
- Create: `internal/gateway/dingtalk/callback_types.go`
- Create: `internal/gateway/dingtalk/app_support.go`
- Create: `internal/gateway/dingtalk/app_mapper.go`
- Create: `internal/gateway/dingtalk/app_handler.go`
- Test: `internal/gateway/dingtalk/app_handler_test.go`

- [ ] **Step 1: Write the failing app handler tests**

```go
func TestAppHandlerRespondsToChallenge(t *testing.T) {}
func TestAppHandlerMapsTextAndEventCallbacks(t *testing.T) {}
func TestAppHandlerMapsImageAudioVideoAndFileCallbacks(t *testing.T) {}
func TestAppHandlerMapsCardActionCallback(t *testing.T) {}
func TestAppHandlerReturnsStatusCodesForMalformedCallbacks(t *testing.T) {}
```

- [ ] **Step 2: Run the focused app handler tests to verify failure**

Run: `go test ./internal/gateway/dingtalk -run 'TestAppHandlerRespondsToChallenge|TestAppHandlerMapsTextAndEventCallbacks|TestAppHandlerMapsImageAudioVideoAndFileCallbacks|TestAppHandlerMapsCardActionCallback|TestAppHandlerReturnsStatusCodesForMalformedCallbacks' -v`

Expected: FAIL because no DingTalk callback handler or mapper exists yet.

- [ ] **Step 3: Write the minimal inbound implementation**

Implement:

```go
func mapEventCallback(body []byte) (gateway.ChannelRequest, error) {}
func mapCardCallback(body []byte) (gateway.ChannelRequest, error) {}
func (a *AppAdapter) EventHandler(router *gateway.SessionRouter) http.Handler {}
func (a *AppAdapter) CardHandler(router *gateway.SessionRouter) http.Handler {}
```

Behavior:

- respond to DingTalk URL verification / challenge requests
- verify callback signature and decrypt payload when required by config
- map `conversationId` to `SessionID`
- preserve `msgId`, `conversationId`, `chatbotUserId`, `senderStaffId`, `eventType`, and `cardCallbackId` in metadata
- convert card actions to `MessageTypeEvent` with `Text = "card_action"`
- populate `RawContent` with the original callback body and `Card` with parsed action content
- return `400/401/502` on malformed, unauthorized, or enqueue-failed callbacks

- [ ] **Step 4: Run the focused app handler tests to verify success**

Run: `go test ./internal/gateway/dingtalk -run 'TestAppHandlerRespondsToChallenge|TestAppHandlerMapsTextAndEventCallbacks|TestAppHandlerMapsImageAudioVideoAndFileCallbacks|TestAppHandlerMapsCardActionCallback|TestAppHandlerReturnsStatusCodesForMalformedCallbacks' -v`

Expected: PASS

### Task 4: Implement DingTalk app outbound sending with the official SDK

**Files:**
- Modify: `go.mod`
- Modify: `go.sum`
- Create: `internal/gateway/dingtalk/app_client.go`
- Create: `internal/gateway/dingtalk/app_adapter.go`
- Test: `internal/gateway/dingtalk/app_client_test.go`

- [ ] **Step 1: Write the failing app client tests**

```go
func TestAppClientSendsTextMessage(t *testing.T) {}
func TestAppClientSendsMarkdownMessage(t *testing.T) {}
func TestAppClientSendsCardMessage(t *testing.T) {}
func TestAppClientUploadsMediaBeforeSending(t *testing.T) {}
func TestAppAdapterSendFallsBackToTextHelper(t *testing.T) {}
func TestAppClientRejectsMissingReceiveTarget(t *testing.T) {}
```

- [ ] **Step 2: Run the focused app client tests to verify failure**

Run: `go test ./internal/gateway/dingtalk -run 'TestAppClientSendsTextMessage|TestAppClientSendsMarkdownMessage|TestAppClientSendsCardMessage|TestAppClientUploadsMediaBeforeSending|TestAppAdapterSendFallsBackToTextHelper|TestAppClientRejectsMissingReceiveTarget' -v`

Expected: FAIL because no DingTalk app client or adapter exists yet.

- [ ] **Step 3: Write the minimal app client implementation**

Add the official dependency:

```bash
go get github.com/alibabacloud-go/dingtalk@latest
```

Implement:

```go
type AppConfig struct {
    ClientID          string
    ClientSecret      string
    AgentID           int64
    EventCallbackPath string
    CardCallbackPath  string
    APIBaseURL        string
    OAPIBaseURL       string
    Token             string
    AESKey            string
    MediaDir          string
    HTTPTimeout       time.Duration
}

func (c *AppClient) Send(ctx context.Context, msg gateway.ChannelResponse) error {}
func (a *AppAdapter) Send(ctx context.Context, msg gateway.ChannelResponse) error {}
```

Implementation rules:

- wrap the official DingTalk SDK behind small local interfaces so tests can stub send/upload behavior
- resolve receive target from `SessionID` first, then well-defined metadata keys such as `dingtalk_conversation_id`
- send text / markdown / card directly
- upload image / audio / video / file when only `Path` or `DataBase64` is provided
- reuse `MediaID` directly when already present
- return explicit errors for unsupported or incomplete message shapes

- [ ] **Step 4: Run the focused app client tests to verify success**

Run: `go test ./internal/gateway/dingtalk -run 'TestAppClientSendsTextMessage|TestAppClientSendsMarkdownMessage|TestAppClientSendsCardMessage|TestAppClientUploadsMediaBeforeSending|TestAppAdapterSendFallsBackToTextHelper|TestAppClientRejectsMissingReceiveTarget' -v`

Expected: PASS

### Task 5: Add DingTalk runbook and env documentation

**Files:**
- Create: `docs/gateway_dingtalk_runbook.md`
- Modify: `docs/README.md`
- Modify: `cmd/gateway/.env.example`
- Modify: `docs/superpowers/specs/2026-03-18-dingtalk-gateway-design.md`
- Modify: `docs/superpowers/plans/2026-03-18-dingtalk-gateway-integration.md`

- [ ] **Step 1: Write the failing documentation checklist**

Document:

- all `DINGTALK_*` env vars
- callback URL paths
- robot webhook examples
- unified ingress example for `channel_name=dingtalk_robot`
- local verification steps

- [ ] **Step 2: Write the runbook and index updates**

Add:

- a DingTalk runbook linked from `docs/README.md`
- `curl` examples for app callback simulation and robot direct-send
- expected `cmd/gateway/.env.example` values

- [ ] **Step 3: Review the docs for consistency with the implemented adapter names**

Verify that every document consistently uses:

```text
dingtalk_app
dingtalk_robot
/callbacks/dingtalk/events
/callbacks/dingtalk/cards
```

Expected: no stale `wecom` / `feishu` names copied into DingTalk examples.

### Task 6: Verify the full DingTalk gateway slice

**Files:**
- Modify: `internal/gateway/config_test.go`
- Modify: `cmd/gateway/main_test.go`
- Modify: `internal/gateway/dingtalk/app_handler_test.go`
- Modify: `internal/gateway/dingtalk/app_client_test.go`
- Modify: `internal/gateway/dingtalk/robot_client_test.go`

- [ ] **Step 1: Run the focused DingTalk package tests**

Run: `go test ./internal/gateway/dingtalk -v`

Expected: PASS

- [ ] **Step 2: Run the shared gateway regression slice**

Run: `go test ./internal/gateway ./cmd/gateway -run 'Config|Gateway|StreamListener|Ingress' -v`

Expected: PASS with existing `wecom` / `feishu` behavior preserved.

- [ ] **Step 3: Run the broader gateway package regression**

Run: `go test ./internal/gateway/... ./cmd/gateway/... -v`

Expected: PASS for all gateway adapters, handlers, and config flows.

- [ ] **Step 4: Record any unavoidable protocol limitations explicitly**

If DingTalk robot webhook cannot send some media classes after protocol verification, update the runbook and tests to assert a clear error contract rather than silent downgrade.

Expected: the final implementation either supports a message type or rejects it explicitly and documents the limitation.
