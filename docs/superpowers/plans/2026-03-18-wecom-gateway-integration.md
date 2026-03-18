# WeCom Gateway Integration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add Enterprise WeChat callback receive + async reply support on top of the existing gateway unified interface, covering text, markdown, image, audio, video, file, and rich media envelopes.

**Architecture:** Introduce a richer `gateway.StandardMessage` while preserving the old plain-text adapter contract, fix router result binding with an in-memory route store, and add a lightweight in-repo WeCom protocol client/handler instead of pulling a third-party SDK. Ship a dedicated `cmd/gateway` entrypoint with centralized env/flag config.

**Tech Stack:** Go 1.21+, stdlib crypto/xml/http, existing Redis transport, existing gateway queue model.

---

### Task 1: Define unified gateway message model

**Files:**
- Create: `internal/gateway/types.go`
- Modify: `internal/gateway/router.go`
- Test: `internal/gateway/router_test.go`

- [ ] **Step 1: Write the failing router tests**

```go
func TestHandleIncomingTracksTaskRouteAndEnqueuesUnifiedPayload(t *testing.T) {}
func TestStartStreamListenerRoutesRichMessageToRegisteredAdapter(t *testing.T) {}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/gateway -run 'TestHandleIncomingTracksTaskRouteAndEnqueuesUnifiedPayload|TestStartStreamListenerRoutesRichMessageToRegisteredAdapter' -v`
Expected: FAIL because route binding and rich message routing do not exist yet.

- [ ] **Step 3: Write minimal implementation**

Add `StandardMessage`, `MediaItem`, `Article`, `RichAdapter`, and a route-binding store in `SessionRouter`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/gateway -run 'TestHandleIncomingTracksTaskRouteAndEnqueuesUnifiedPayload|TestStartStreamListenerRoutesRichMessageToRegisteredAdapter' -v`
Expected: PASS

### Task 2: Add WeCom callback crypto and parsing

**Files:**
- Create: `internal/gateway/wecom/crypto.go`
- Create: `internal/gateway/wecom/xml_types.go`
- Create: `internal/gateway/wecom/handler.go`
- Test: `internal/gateway/wecom/crypto_test.go`
- Test: `internal/gateway/wecom/handler_test.go`

- [ ] **Step 1: Write the failing crypto and handler tests**

```go
func TestCodecVerifyURLRoundTrip(t *testing.T) {}
func TestHandlerParsesTextAndMediaCallbacksIntoStandardMessage(t *testing.T) {}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/gateway/wecom -run 'TestCodecVerifyURLRoundTrip|TestHandlerParsesTextAndMediaCallbacksIntoStandardMessage' -v`
Expected: FAIL because no WeCom callback codec/handler exists yet.

- [ ] **Step 3: Write minimal implementation**

Implement signature validation, AES-CBC decode, XML parsing, and callback-to-`StandardMessage` conversion.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/gateway/wecom -run 'TestCodecVerifyURLRoundTrip|TestHandlerParsesTextAndMediaCallbacksIntoStandardMessage' -v`
Expected: PASS

### Task 3: Add WeCom outbound client and adapter

**Files:**
- Create: `internal/gateway/wecom/client.go`
- Create: `internal/gateway/wecom/config.go`
- Create: `internal/gateway/wecom/client_test.go`

- [ ] **Step 1: Write the failing client tests**

```go
func TestAdapterSendUsesMarkdownPayload(t *testing.T) {}
func TestAdapterSendUploadsMediaBeforeSending(t *testing.T) {}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/gateway/wecom -run 'TestAdapterSendUsesMarkdownPayload|TestAdapterSendUploadsMediaBeforeSending' -v`
Expected: FAIL because outbound token/send/upload flow is missing.

- [ ] **Step 3: Write minimal implementation**

Add access-token caching, `message/send`, `media/upload`, and unified response mapping.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/gateway/wecom -run 'TestAdapterSendUsesMarkdownPayload|TestAdapterSendUploadsMediaBeforeSending' -v`
Expected: PASS

### Task 4: Add centralized gateway config and entrypoint

**Files:**
- Create: `internal/gateway/config.go`
- Create: `internal/gateway/config_test.go`
- Create: `cmd/gateway/main.go`

- [ ] **Step 1: Write the failing config tests**

```go
func TestParseConfigUsesEnvAndFlags(t *testing.T) {}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/gateway -run TestParseConfigUsesEnvAndFlags -v`
Expected: FAIL because gateway config parsing does not exist yet.

- [ ] **Step 3: Write minimal implementation**

Add `gateway.Config`, nested `wecom.Config`, defaults, validation, and `cmd/gateway` wiring.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/gateway -run TestParseConfigUsesEnvAndFlags -v`
Expected: PASS

### Task 5: Verify end-to-end gateway slice

**Files:**
- Modify: `internal/gateway/router.go`
- Modify: `internal/gateway/mock_adapter.go`
- Modify: `docs/superpowers/specs/2026-03-18-wecom-gateway-design.md`

- [ ] **Step 1: Run focused package tests**

Run: `go test ./internal/gateway ./internal/gateway/wecom -v`
Expected: PASS

- [ ] **Step 2: Run gateway command compile check**

Run: `go test ./cmd/gateway -v`
Expected: PASS

- [ ] **Step 3: Run broader regression slice**

Run: `go test ./internal/gateway ./cmd/orchestrator ./cmd/subagent -run 'Gateway|Config|Sender|ChatCompletions' -v`
Expected: PASS for touched gateway-adjacent flows
