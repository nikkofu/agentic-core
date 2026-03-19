# Unified Logging Foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a unified project-wide logging foundation that prints clearly in the terminal, writes structured JSON logs to local files, rotates by date, keeps 30 days by default, supports log levels, and produces LLM-friendly output.

**Architecture:** Introduce a shared `internal/logging` package built on Go's `log/slog`. A custom daily rotating writer will fan out to console and JSONL files under `logs/YYYY-MM-DD/`, while keeping retention cleanup and component-scoped structured fields in one place. Existing `fmt.Printf` and ad-hoc audit writes in the critical path will be migrated first to reduce risk and preserve behavior.

**Tech Stack:** Go `log/slog`, standard library file I/O/time/path APIs, existing `internal/process/audit.go`, existing command entrypoints.

---

## File Map

- Create: `internal/logging/config.go` — logging config defaults, level parsing, directory policy.
- Create: `internal/logging/manager.go` — global setup, component logger factory, default logger wiring.
- Create: `internal/logging/daily_writer.go` — date-based file writer, retention cleanup, concurrency safety.
- Create: `internal/logging/console_handler.go` — human-readable terminal formatting helper/wrapper.
- Create: `internal/logging/logging_test.go` — unit tests for config, level filtering, logger wiring.
- Create: `internal/logging/daily_writer_test.go` — unit tests for day split and retention cleanup.
- Modify: `internal/process/audit.go` — route audit persistence through shared logging backend instead of raw file append.
- Modify: `cmd/orchestrator/main.go` — initialize component logger and replace critical `fmt.Printf` usage.
- Modify: `cmd/subagent/main.go` — initialize component logger and replace critical `fmt.Printf` usage.
- Modify: `internal/gateway/router.go` — replace gateway `fmt.Printf` with structured logger.
- Modify: `internal/gateway/mock_adapter.go` — replace mock adapter `fmt.Printf` with structured logger.
- Test: `cmd/orchestrator/main_test.go` — cover initialization/behavior where logger integration changes semantics.
- Test: `cmd/subagent/main_test.go` — cover logger-backed chunk/audit path if needed.

### Task 1: Build config and daily writer primitives

**Files:**
- Create: `internal/logging/config.go`
- Create: `internal/logging/daily_writer.go`
- Create: `internal/logging/daily_writer_test.go`

- [ ] **Step 1: Write the failing tests for daily split and retention**

```go
func TestDailyWriterSplitsByDate(t *testing.T) {}
func TestDailyWriterRemovesExpiredDirectories(t *testing.T) {}
func TestParseLevelDefaultsToInfo(t *testing.T) {}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/logging -run 'Test(DailyWriter|ParseLevel)' -count=1`
Expected: FAIL because package/files do not exist yet.

- [ ] **Step 3: Write minimal config and writer implementation**

```go
type Config struct { Dir string; Level slog.Level; RetentionDays int; JSONFile bool }
func ParseLevel(raw string) slog.Level { ... }
func (w *DailyWriter) Write(p []byte) (int, error) { ... }
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/logging -run 'Test(DailyWriter|ParseLevel)' -count=1`
Expected: PASS

### Task 2: Build unified logger manager and output fanout

**Files:**
- Create: `internal/logging/manager.go`
- Create: `internal/logging/console_handler.go`
- Create: `internal/logging/logging_test.go`
- Modify: `internal/logging/daily_writer.go`

- [ ] **Step 1: Write the failing tests for dual output and level filtering**

```go
func TestManagerWritesJSONLineToFile(t *testing.T) {}
func TestManagerFiltersDebugBelowInfo(t *testing.T) {}
func TestComponentLoggerAddsComponentField(t *testing.T) {}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/logging -run 'Test(Manager|ComponentLogger)' -count=1`
Expected: FAIL because manager/handlers are incomplete.

- [ ] **Step 3: Write minimal manager and handlers**

```go
func Init(cfg Config) (*Manager, error) { ... }
func (m *Manager) Component(name string) *slog.Logger { ... }
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/logging -run 'Test(Manager|ComponentLogger)' -count=1`
Expected: PASS

### Task 3: Route audit logging through the shared backend

**Files:**
- Modify: `internal/process/audit.go`
- Modify: `internal/logging/manager.go`
- Test: `internal/logging/logging_test.go`

- [ ] **Step 1: Write the failing test for audit integration**

```go
func TestAuditorRecordWritesStructuredJSONLog(t *testing.T) {}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/logging ./internal/process -run 'TestAuditorRecordWritesStructuredJSONLog' -count=1`
Expected: FAIL because auditor is not using the shared backend.

- [ ] **Step 3: Implement minimal audit backend integration**

```go
func NewAuditor(events bus.EventBus, senderID string) *Auditor { ... }
func (a *Auditor) Record(ctx context.Context, event llm.AuditEvent) error { ... }
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/logging ./internal/process -run 'TestAuditorRecordWritesStructuredJSONLog' -count=1`
Expected: PASS

### Task 4: Wire orchestrator and subagent to the unified logger

**Files:**
- Modify: `cmd/orchestrator/main.go`
- Modify: `cmd/subagent/main.go`
- Test: `cmd/orchestrator/main_test.go`
- Test: `cmd/subagent/main_test.go`

- [ ] **Step 1: Write the failing tests for logger initialization and component output**

```go
func TestNewAppInitializesLogger(t *testing.T) {}
func TestNewSubagentInitializesLogger(t *testing.T) {}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./cmd/orchestrator ./cmd/subagent -run 'Test(NewAppInitializesLogger|NewSubagentInitializesLogger)' -count=1`
Expected: FAIL because command structs do not own a shared logger yet.

- [ ] **Step 3: Implement minimal command wiring and replace critical prints**

```go
type App struct { logger *slog.Logger }
type Subagent struct { logger *slog.Logger }
logger.Info("task routed", ...)
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./cmd/orchestrator ./cmd/subagent -run 'Test(NewAppInitializesLogger|NewSubagentInitializesLogger)' -count=1`
Expected: PASS

### Task 5: Migrate gateway prints and run focused regression

**Files:**
- Modify: `internal/gateway/router.go`
- Modify: `internal/gateway/mock_adapter.go`
- Test: `internal/gateway/sender_test.go`
- Test: `internal/gateway/chat_completions_handler_test.go`

- [ ] **Step 1: Write the failing tests for gateway logger usage if behavior changes**

```go
func TestGatewayUsesStructuredLoggerWithoutBreakingRouting(t *testing.T) {}
```

- [ ] **Step 2: Run tests to verify they fail or expose missing wiring**

Run: `go test ./internal/gateway -count=1`
Expected: FAIL or missing logger wiring for gateway components.

- [ ] **Step 3: Implement minimal gateway logging migration**

```go
logger.Info("incoming channel request", ...)
logger.Info("mock adapter send", ...)
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/gateway -count=1`
Expected: PASS

### Task 6: Verify integrated logging behavior

**Files:**
- Modify: `docs/README.md` (only if logging env/config needs documenting)
- Test: `internal/logging/*.go`
- Test: `cmd/orchestrator/*.go`
- Test: `cmd/subagent/*.go`
- Test: `internal/gateway/*.go`

- [ ] **Step 1: Run focused package tests**

Run: `go test ./internal/logging ./internal/process ./cmd/orchestrator ./cmd/subagent ./internal/gateway -count=1`
Expected: PASS

- [ ] **Step 2: Run full regression suite**

Run: `go test ./... -timeout 30s`
Expected: PASS

- [ ] **Step 3: Document runtime knobs if needed**

```md
- `LOG_DIR`
- `LOG_LEVEL`
- `LOG_RETENTION_DAYS`
```

- [ ] **Step 4: Re-run final verification**

Run: `go test ./... -timeout 30s`
Expected: PASS
