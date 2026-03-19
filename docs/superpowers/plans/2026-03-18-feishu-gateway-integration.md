# Feishu Gateway Integration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add China-region Feishu gateway support with both self-built app event subscription and group bot webhook sending, while preserving existing `wecom` behavior and extending the shared gateway layer only where required.

**Architecture:** Extend the shared `gateway.StandardMessage` and `SessionRouter` with card payloads and a direct-send path, then add a new `internal/gateway/feishu` package split into `app` and `bot` responsibilities. Use the official Feishu Go SDK for self-built app API/event handling, use a lightweight stdlib webhook client for group bots, and gate all startup/config validation by enabled adapter.

**Tech Stack:** Go 1.25.x, existing Redis transport, existing gateway queue model, official Feishu SDK `github.com/larksuite/oapi-sdk-go/v3`, stdlib `net/http`/`crypto/hmac` for bot webhook signing, existing `go test`.

---

## File Map

- `internal/gateway/types.go`
  - Extend the unified message model with card/raw payload fields while preserving current `wecom` consumers.
- `internal/gateway/router.go`
  - Add direct-send result routing when `task_results.message.channel_name` is present.
- `internal/gateway/router_test.go`
  - Cover route-bound replies and direct-send replies together.
- `internal/gateway/config.go`
  - Replace the current “always require WeCom” validation with adapter-aware validation.
- `internal/gateway/config_test.go`
  - Verify selective enablement and mixed adapter config parsing.
- `cmd/gateway/main.go`
  - Conditionally register `wecom`, `feishu_app`, and `feishu_bot`.
- `internal/gateway/feishu/config.go`
  - Hold Feishu app/bot config defaults and validation helpers.
- `internal/gateway/feishu/app_mapper.go`
  - Convert Feishu events and card callbacks into `gateway.ChannelRequest`.
- `internal/gateway/feishu/app_handler.go`
  - Handle event subscription and card callback HTTP requests.
- `internal/gateway/feishu/app_adapter.go`
  - Provide `gateway.RichAdapter` behavior for `feishu_app`.
- `internal/gateway/feishu/app_client.go`
  - Wrap the official Feishu SDK for send/upload operations.
- `internal/gateway/feishu/bot_adapter.go`
  - Provide `gateway.RichAdapter` behavior for `feishu_bot`.
- `internal/gateway/feishu/bot_client.go`
  - Send webhook text/card payloads and compute optional signatures.
- `internal/gateway/feishu/bot_payload.go`
  - Build webhook payloads from `gateway.ChannelResponse`.
- `internal/gateway/feishu/app_handler_test.go`
  - Test challenge, message callback mapping, and card callback mapping.
- `internal/gateway/feishu/app_client_test.go`
  - Test app-side send payload mapping and upload-then-send flow with stubs.
- `internal/gateway/feishu/bot_client_test.go`
  - Test webhook payloads and signing.

### Task 1: Extend unified gateway message routing

**Files:**
- Modify: `internal/gateway/types.go`
- Modify: `internal/gateway/router.go`
- Test: `internal/gateway/router_test.go`

- [ ] **Step 1: Write the failing router tests**

```go
func TestStartStreamListenerDirectSendUsesExplicitChannelMessage(t *testing.T) {}
func TestStartStreamListenerDirectSendFallsBackToLegacyRouteBinding(t *testing.T) {}
```

- [ ] **Step 2: Run the focused router tests and verify failure**

Run: `go test ./internal/gateway -run 'TestStartStreamListenerDirectSendUsesExplicitChannelMessage|TestStartStreamListenerDirectSendFallsBackToLegacyRouteBinding|TestHandleIncomingTracksTaskRouteAndEnqueuesUnifiedPayload|TestStartStreamListenerRoutesRichMessageToRegisteredAdapter' -v`

Expected: FAIL because the current router only knows route binding and `StandardMessage` has no card/raw payload fields.

- [ ] **Step 3: Write the minimal shared message and router implementation**

Implement:

```go
type StandardMessage struct {
    // existing fields...
    Card       map[string]any   `json:"card,omitempty"`
    RawContent json.RawMessage  `json:"raw_content,omitempty"`
}
```

And in `router.go`:

```go
if envelope.Message != nil && envelope.Message.ChannelName != "" {
    // direct-send path: use explicit channel without route binding
}
```

- [ ] **Step 4: Run the focused router tests and verify success**

Run: `go test ./internal/gateway -run 'TestStartStreamListenerDirectSendUsesExplicitChannelMessage|TestStartStreamListenerDirectSendFallsBackToLegacyRouteBinding|TestHandleIncomingTracksTaskRouteAndEnqueuesUnifiedPayload|TestStartStreamListenerRoutesRichMessageToRegisteredAdapter' -v`

Expected: PASS

### Task 2: Add adapter-aware gateway config

**Files:**
- Modify: `internal/gateway/config.go`
- Test: `internal/gateway/config_test.go`

- [ ] **Step 1: Write the failing config tests**

```go
func TestParseConfigAllowsFeishuOnlyMode(t *testing.T) {}
func TestParseConfigAllowsBotOnlyMode(t *testing.T) {}
func TestParseConfigRejectsNoEnabledAdapter(t *testing.T) {}
```

- [ ] **Step 2: Run the focused config tests and verify failure**

Run: `go test ./internal/gateway -run 'TestParseConfigUsesEnvAndFlags|TestParseConfigAllowsFeishuOnlyMode|TestParseConfigAllowsBotOnlyMode|TestParseConfigRejectsNoEnabledAdapter' -v`

Expected: FAIL because the current config always requires `WECOM_*` values.

- [ ] **Step 3: Write the minimal config implementation**

Add:

```go
type FeishuAppConfig struct { /* env/flag-backed fields */ }
type FeishuBotConfig struct { /* env/flag-backed fields */ }

type Config struct {
    HTTPPort   string
    RedisAddr  string
    WeCom      WeComConfig
    FeishuApp  FeishuAppConfig
    FeishuBot  FeishuBotConfig
}
```

Validation rules:

- At least one adapter enabled
- Validate only enabled adapters
- Preserve current `wecom` names and defaults

- [ ] **Step 4: Run the focused config tests and verify success**

Run: `go test ./internal/gateway -run 'TestParseConfigUsesEnvAndFlags|TestParseConfigAllowsFeishuOnlyMode|TestParseConfigAllowsBotOnlyMode|TestParseConfigRejectsNoEnabledAdapter' -v`

Expected: PASS

### Task 3: Implement Feishu bot webhook sending

**Files:**
- Create: `internal/gateway/feishu/config.go`
- Create: `internal/gateway/feishu/bot_payload.go`
- Create: `internal/gateway/feishu/bot_client.go`
- Create: `internal/gateway/feishu/bot_adapter.go`
- Test: `internal/gateway/feishu/bot_client_test.go`

- [ ] **Step 1: Write the failing bot tests**

```go
func TestBotClientBuildsSignedTextPayload(t *testing.T) {}
func TestBotClientBuildsCardPayload(t *testing.T) {}
func TestBotAdapterSendUsesWebhookClient(t *testing.T) {}
```

- [ ] **Step 2: Run the focused bot tests and verify failure**

Run: `go test ./internal/gateway/feishu -run 'TestBotClientBuildsSignedTextPayload|TestBotClientBuildsCardPayload|TestBotAdapterSendUsesWebhookClient' -v`

Expected: FAIL because no Feishu bot implementation exists yet.

- [ ] **Step 3: Write the minimal bot implementation**

Implement:

```go
type BotConfig struct {
    Enabled    bool
    WebhookURL string
    Secret     string
}

func buildBotPayload(msg gateway.ChannelResponse, ts int64, secret string) (map[string]any, error) {}
func (c *BotClient) Send(ctx context.Context, msg gateway.ChannelResponse) error {}
func (a *BotAdapter) Send(ctx context.Context, msg gateway.ChannelResponse) error {}
```

Mapping rules:

- `MessageTypeText` → webhook `text`
- `FormatMarkdown` or `MessageTypeMarkdown` → webhook `post`
- `Card != nil` → webhook `interactive`

- [ ] **Step 4: Run the focused bot tests and verify success**

Run: `go test ./internal/gateway/feishu -run 'TestBotClientBuildsSignedTextPayload|TestBotClientBuildsCardPayload|TestBotAdapterSendUsesWebhookClient' -v`

Expected: PASS

### Task 4: Implement Feishu app inbound event mapping

**Files:**
- Create: `internal/gateway/feishu/app_mapper.go`
- Create: `internal/gateway/feishu/app_handler.go`
- Test: `internal/gateway/feishu/app_handler_test.go`

- [ ] **Step 1: Write the failing app handler tests**

```go
func TestAppHandlerRespondsToChallenge(t *testing.T) {}
func TestAppHandlerMapsTextMessageEvent(t *testing.T) {}
func TestAppHandlerMapsImageAndFileMessageEvents(t *testing.T) {}
func TestAppHandlerMapsCardActionEvent(t *testing.T) {}
func TestAppHandlerReturnsStatusCodesForMalformedCallbacks(t *testing.T) {}
func TestAppHandlerLogsSafeCallbackMetadata(t *testing.T) {}
```

- [ ] **Step 2: Run the focused app handler tests and verify failure**

Run: `go test ./internal/gateway/feishu -run 'TestAppHandlerRespondsToChallenge|TestAppHandlerMapsTextMessageEvent|TestAppHandlerMapsImageAndFileMessageEvents|TestAppHandlerMapsCardActionEvent|TestAppHandlerReturnsStatusCodesForMalformedCallbacks|TestAppHandlerLogsSafeCallbackMetadata' -v`

Expected: FAIL because no Feishu app event handler or mapper exists yet.

- [ ] **Step 3: Write the minimal inbound implementation**

Implement:

```go
func mapMessageEvent(body []byte) (gateway.ChannelRequest, error) {}
func mapCardCallback(body []byte) (gateway.ChannelRequest, error) {}
func (a *AppAdapter) EventHandler(router *gateway.SessionRouter) http.Handler {}
func (a *AppAdapter) CardHandler(router *gateway.SessionRouter) http.Handler {}
```

Behavior:

- respond to `challenge`
- map `chat_id` to `SessionID`
- preserve `message_id`, `event_id`, `open_id`, `chat_id`, `tenant_key` in metadata
- convert card actions to `MessageTypeEvent`
- return `400/401/502` on malformed, unauthorized, or enqueue-failed callbacks
- emit logs with adapter name, callback kind, session/chat/message identifiers, and platform error info without logging secrets or raw encrypted payloads

- [ ] **Step 4: Run the focused app handler tests and verify success**

Run: `go test ./internal/gateway/feishu -run 'TestAppHandlerRespondsToChallenge|TestAppHandlerMapsTextMessageEvent|TestAppHandlerMapsImageAndFileMessageEvents|TestAppHandlerMapsCardActionEvent|TestAppHandlerReturnsStatusCodesForMalformedCallbacks|TestAppHandlerLogsSafeCallbackMetadata' -v`

Expected: PASS

### Task 5: Implement Feishu app outbound sending

**Files:**
- Create: `internal/gateway/feishu/app_client.go`
- Create: `internal/gateway/feishu/app_adapter.go`
- Test: `internal/gateway/feishu/app_client_test.go`

- [ ] **Step 1: Write the failing app client tests**

```go
func TestAppClientSendsTextMessageToChatID(t *testing.T) {}
func TestAppClientSendsCardMessage(t *testing.T) {}
func TestAppClientUploadsImageBeforeSending(t *testing.T) {}
func TestAppAdapterSendFallsBackToTextHelper(t *testing.T) {}
func TestAppClientLogsNonSensitiveSendFailureMetadata(t *testing.T) {}
```

- [ ] **Step 2: Run the focused app client tests and verify failure**

Run: `go test ./internal/gateway/feishu -run 'TestAppClientSendsTextMessageToChatID|TestAppClientSendsCardMessage|TestAppClientUploadsImageBeforeSending|TestAppAdapterSendFallsBackToTextHelper|TestAppClientLogsNonSensitiveSendFailureMetadata' -v`

Expected: FAIL because the SDK-backed Feishu app client does not exist yet.

- [ ] **Step 3: Write the minimal outbound implementation**

Implement:

```go
type AppClient struct {
    // wrap official SDK methods behind an interface for tests
}

func (c *AppClient) Send(ctx context.Context, msg gateway.ChannelResponse) error {}
func (c *AppClient) uploadMediaIfNeeded(ctx context.Context, media gateway.MediaItem) (string, error) {}
func (a *AppAdapter) Send(ctx context.Context, msg gateway.ChannelResponse) error {}
```

Mapping rules:

- `MessageTypeText` → text
- `FormatMarkdown` / `MessageTypeMarkdown` → post or text-as-markdown path chosen once and used consistently in tests
- `MessageTypeImage` / `MessageTypeFile` → upload if needed, then send media message
- `Card != nil` → interactive card message
- on send/upload failure, log adapter name, receive id type, session target, and upstream request identifiers without leaking app secret, verification token, encrypt key, or raw file bytes

- [ ] **Step 4: Run the focused app client tests and verify success**

Run: `go test ./internal/gateway/feishu -run 'TestAppClientSendsTextMessageToChatID|TestAppClientSendsCardMessage|TestAppClientUploadsImageBeforeSending|TestAppAdapterSendFallsBackToTextHelper|TestAppClientLogsNonSensitiveSendFailureMetadata' -v`

Expected: PASS

### Task 6: Wire Feishu adapters into the gateway entrypoint

**Files:**
- Modify: `cmd/gateway/main.go`
- Modify: `internal/gateway/config.go`
- Modify: `internal/gateway/router.go`
- Test: `internal/gateway/config_test.go`
- Test: `internal/gateway/router_test.go`
- Test: `cmd/gateway/main_test.go` (create only if needed for wiring coverage)

- [ ] **Step 1: Write the failing integration tests**

```go
func TestGatewayCanInitializeWithFeishuOnlyConfig(t *testing.T) {}
func TestGatewayRegistersFeishuHandlersWhenEnabled(t *testing.T) {}
```

- [ ] **Step 2: Run the focused integration tests and verify failure**

Run: `go test ./cmd/gateway ./internal/gateway -run 'TestGatewayCanInitializeWithFeishuOnlyConfig|TestGatewayRegistersFeishuHandlersWhenEnabled' -v`

Expected: FAIL because `cmd/gateway` only wires `wecom`.

- [ ] **Step 3: Write the minimal wiring implementation**

Update `cmd/gateway/main.go` to:

- create and register `wecom` only when enabled
- create and register `feishu_app` when enabled
- create and register `feishu_bot` when enabled
- mount Feishu event/card callback paths
- still start one shared `SessionRouter`

- [ ] **Step 4: Run the focused integration tests and verify success**

Run: `go test ./cmd/gateway ./internal/gateway -run 'TestGatewayCanInitializeWithFeishuOnlyConfig|TestGatewayRegistersFeishuHandlersWhenEnabled' -v`

Expected: PASS

### Task 7: Verify the gateway slice end-to-end

**Files:**
- Modify: `docs/superpowers/specs/2026-03-18-feishu-gateway-design.md`
- Modify: `docs/superpowers/plans/2026-03-18-feishu-gateway-integration.md`

- [ ] **Step 1: Run focused gateway package tests**

Run: `go test ./internal/gateway ./internal/gateway/feishu -v`

Expected: PASS

- [ ] **Step 2: Run gateway command compile verification**

Run: `go test ./cmd/gateway -v`

Expected: PASS

- [ ] **Step 3: Run adjacent regression slice**

Run: `go test ./internal/gateway ./cmd/orchestrator ./cmd/subagent -run 'Gateway|Config|Sender|ChatCompletions' -v`

Expected: PASS for touched gateway-adjacent flows

- [ ] **Step 4: Document any SDK/module bootstrap needed**

Record:

- added Go module dependency
- any env vars required for local verification
- any known limitations kept intentionally out of scope
