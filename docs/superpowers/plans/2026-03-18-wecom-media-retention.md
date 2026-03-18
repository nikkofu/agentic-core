# WeCom Media Retention Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add configurable retention cleanup for files downloaded into `WECOM_MEDIA_DIR` without changing the existing unified gateway send/receive interface.

**Architecture:** Keep media retention isolated from callback parsing and outbound sending. Parse retention days centrally in `internal/gateway/config.go`, then start a lightweight WeCom media janitor from `cmd/gateway/main.go` that immediately sweeps expired files and continues on a fixed background interval.

**Tech Stack:** Go 1.21+, stdlib `os`, `filepath`, `time`, existing gateway config and WeCom adapter packages.

---

### Task 1: Parse media retention settings

**Files:**
- Modify: `internal/gateway/config.go`
- Modify: `internal/gateway/config_test.go`

- [ ] **Step 1: Write the failing config test**

```go
func TestParseConfigUsesMediaRetentionDays(t *testing.T) {}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/gateway -run TestParseConfigUsesMediaRetentionDays -v`
Expected: FAIL because the retention field is not parsed yet.

- [ ] **Step 3: Write minimal implementation**

Add `WECOM_MEDIA_RETENTION_DAYS` parsing and flag support while keeping existing config validation unchanged.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/gateway -run TestParseConfigUsesMediaRetentionDays -v`
Expected: PASS

### Task 2: Add WeCom media cleanup behavior

**Files:**
- Create: `internal/gateway/wecom/media_retention.go`
- Create: `internal/gateway/wecom/media_retention_test.go`

- [ ] **Step 1: Write the failing cleanup tests**

```go
func TestCleanupExpiredMediaFilesRemovesExpiredFiles(t *testing.T) {}
func TestStartMediaRetentionRunsImmediateSweep(t *testing.T) {}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/gateway/wecom -run 'TestCleanupExpiredMediaFilesRemovesExpiredFiles|TestStartMediaRetentionRunsImmediateSweep' -v`
Expected: FAIL because the cleanup functions do not exist yet.

- [ ] **Step 3: Write minimal implementation**

Add a cleanup helper that deletes expired files under `WECOM_MEDIA_DIR` and a janitor starter that performs an immediate sweep plus periodic background cleanup.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/gateway/wecom -run 'TestCleanupExpiredMediaFilesRemovesExpiredFiles|TestStartMediaRetentionRunsImmediateSweep' -v`
Expected: PASS

### Task 3: Wire janitor into gateway startup

**Files:**
- Modify: `cmd/gateway/main.go`

- [ ] **Step 1: Write the failing integration expectation**

Rely on the focused janitor test plus compile coverage from `cmd/gateway`.

- [ ] **Step 2: Write minimal implementation**

Start the janitor only when the WeCom app channel is enabled and retention is configured.

- [ ] **Step 3: Run compile and package verification**

Run: `go test ./internal/gateway ./internal/gateway/wecom ./cmd/gateway -v`
Expected: PASS

### Task 4: Document retention settings

**Files:**
- Modify: `cmd/gateway/.env.example`
- Modify: `docs/gateway_wecom_runbook.md`
- Modify: `docs/superpowers/specs/2026-03-18-wecom-gateway-design.md`

- [ ] **Step 1: Update configuration examples**

Document the new retention variable and the janitor behavior.

- [ ] **Step 2: Run full regression verification**

Run: `go test ./...`
Expected: PASS
