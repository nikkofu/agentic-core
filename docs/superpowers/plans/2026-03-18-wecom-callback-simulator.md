# WeCom Callback Simulator Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a local simulator for Enterprise WeChat self-built app callbacks so encrypted callback requests can be generated and sent to `/callbacks/wecom` without using the WeCom admin console.

**Architecture:** Reuse the in-repo WeCom `Codec` to build a valid encrypted callback envelope and signature, then wrap it with a thin shell script that either generates a default text callback or forwards a user-provided inner XML file. Keep protocol logic in Go and keep shell focused on developer ergonomics.

**Tech Stack:** Go 1.21+, stdlib `net/http`, `encoding/xml`, existing `internal/gateway/wecom` crypto implementation, bash helper scripts.

---

### Task 1: Add simulator request builder

**Files:**
- Create: `cmd/wecom-callback-sim/main.go`
- Create: `cmd/wecom-callback-sim/main_test.go`

- [ ] **Step 1: Write the failing builder test**

```go
func TestBuildCallbackRequestProducesDecryptableEnvelope(t *testing.T) {}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/wecom-callback-sim -run TestBuildCallbackRequestProducesDecryptableEnvelope -v`
Expected: FAIL because the simulator package and builder do not exist yet.

- [ ] **Step 3: Write minimal implementation**

Add a small CLI that reads inner XML, encrypts it with `internal/gateway/wecom.Codec`, builds the callback query string, and posts or prints the request.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./cmd/wecom-callback-sim -run TestBuildCallbackRequestProducesDecryptableEnvelope -v`
Expected: PASS

### Task 2: Add callback demo shell helper

**Files:**
- Modify: `scripts/wecom_helpers_test.sh`
- Create: `scripts/wecom_callback_demo.sh`

- [ ] **Step 1: Write the failing helper test**

Extend the shell test suite with a new `test_callback_demo_helper`.

- [ ] **Step 2: Run test to verify it fails**

Run: `bash scripts/wecom_helpers_test.sh`
Expected: FAIL because the callback demo helper does not exist yet.

- [ ] **Step 3: Write minimal implementation**

Add a shell script that generates a default text callback XML when no file argument is passed, then invokes the simulator CLI.

- [ ] **Step 4: Run test to verify it passes**

Run: `bash scripts/wecom_helpers_test.sh`
Expected: PASS

### Task 3: Document callback simulation usage

**Files:**
- Modify: `docs/gateway_wecom_runbook.md`

- [ ] **Step 1: Add callback demo section**

Document required env vars and example commands for both generated text callbacks and custom inner XML files.

- [ ] **Step 2: Run focused verification**

Run: `go test ./internal/gateway ./internal/gateway/wecom ./internal/gateway/wecomrobot ./cmd/gateway ./cmd/wecom-callback-sim -v`
Expected: PASS

- [ ] **Step 3: Run gateway build checks**

Run: `go build ./cmd/gateway && go build ./cmd/wecom-callback-sim`
Expected: PASS
