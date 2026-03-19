# Phase A Documentation Truth Reset and Gold-Path Proof Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Align the repository’s public documentation with the runtime-governance reset, then expose the already-landed gold-path regressions as one explicit, rerunnable proof suite.

**Architecture:** Keep Phase A intentionally narrow. First, correct the top-level documentation entry points so they distinguish implemented, partial, and deferred capabilities without rewriting the whole docs tree. Then add three missing anchor docs plus one new gold-path proof guide, and back that guide with a small shell dispatcher in `scripts/` that runs named proof buckets using existing `go test` regressions and one lightweight smoke slice.

**Tech Stack:** Markdown docs, Bash, Go 1.21+, existing `go test` suites under `cmd/orchestrator` and `internal/gateway`, existing `scripts/` conventions, Git worktree-based execution.

---

## File Map

- Create: `docs/testing/gold_path_proof.md` — human/LLM-readable explanation of proof buckets, exact commands, and coverage boundaries.
- Create: `docs/architecture/process_scheduling.md` — current-state note for process scheduling and orchestration boundaries.
- Create: `docs/protocol/internal_bus.md` — current-state note for queue/event bus contracts and channel semantics.
- Create: `docs/journal/README.md` — journal index placeholder with current purpose and expected future content.
- Create: `scripts/proof_gold_path.sh` — named proof bucket runner with clear terminal output and stable exit codes.
- Modify: `README.md` — reset the repository positioning to “Agent Runtime + Governance Kernel,” and downgrade deferred platform claims.
- Modify: `docs/README.md` — expose the reset plan, backlog, state matrix, and proof entry points as the current doc hub.
- Reference: `docs/roadmap/task_backlog.md` — active backlog defining `BG-001` and `BG-103`.
- Reference: `docs/superpowers/specs/2026-03-19-phase-a-doc-truth-proof-design.md` — approved spec that this plan implements.

### Task 0: Preflight and baseline verification

**Files:**
- Modify: `docs/superpowers/plans/2026-03-19-phase-a-doc-truth-proof.md` (checkbox updates only during execution)

- [ ] **Step 1: Verify the execution branch and current repo state**

Run: `git status --short --branch`
Expected: execution occurs inside the dedicated worktree branch, with only intentional plan-time or runtime-generated artifacts visible.

- [ ] **Step 2: Run the current repository baseline outside the sandbox if needed**

Run: `GOCACHE="$(pwd)/.cache/go-build" go test ./... -timeout 30s`
Expected: PASS. If local listener tests fail because the sandbox cannot bind sockets, rerun the same command with approval outside the sandbox and record that this is an environment constraint, not a repo regression.

- [ ] **Step 3: Record the allowed scope for Phase A**

Keep implementation limited to:
- `README.md`
- `docs/README.md`
- the three missing anchor docs
- `docs/testing/gold_path_proof.md`
- `scripts/proof_gold_path.sh`

Do not expand into audit persistence, sticky session storage, provider refactors, or WASM wiring during this phase.

### Task 1: Reset the root documentation truth

**Files:**
- Modify: `README.md`
- Modify: `docs/README.md`

- [ ] **Step 1: Rewrite the root positioning in `README.md`**

Replace the “enterprise multi-agent platform” top-level framing with a current-phase summary that clearly separates:
- implemented runtime/governance kernel capabilities
- partial capabilities still being hardened
- deferred platform capabilities

Use a structure like:

```md
## 当前阶段定位
## 已实现 / 可证明能力
## 部分实现能力
## 明确延后能力
```

- [ ] **Step 2: Downgrade or remove misleading capability claims**

Edit `README.md` so it no longer reads as if the following are already mainline-complete:
- production RAG / Milvus in the execution path
- full `@agent` collaboration semantics
- WASM as the active execution backend
- production OTEL observability

- [ ] **Step 3: Turn `docs/README.md` into the current execution hub**

Add or refine:
- links to the reset plan, active backlog, and approved Phase A spec
- a small state matrix for `implemented`, `partial`, and `deferred`
- a link to the new `docs/testing/gold_path_proof.md`

Use a structure like:

```md
## 当前执行锚点
## 当前状态矩阵
## 证明入口
```

- [ ] **Step 4: Verify the documentation entry points are present**

Run: `rg -n "Runtime Governance Reset|Task Backlog|gold_path_proof|implemented|partial|deferred" README.md docs/README.md`
Expected: matches in both files showing the new anchors and state language.

- [ ] **Step 5: Review the resulting doc drift manually**

Read: `README.md` and `docs/README.md`
Expected: a new contributor can tell within a few minutes what is real, what is partial, and what is deferred.

- [ ] **Step 6: Commit the top-level documentation reset**

```bash
git add README.md docs/README.md
git commit -m "docs: reset runtime-phase entry points"
```

### Task 2: Add the missing architecture/protocol/journal anchors

**Files:**
- Create: `docs/architecture/process_scheduling.md`
- Create: `docs/protocol/internal_bus.md`
- Create: `docs/journal/README.md`

- [ ] **Step 1: Draft `docs/architecture/process_scheduling.md` as a current-state note**

Cover only what is true today:
- orchestrator / subagent roles
- process spawning boundary
- workflow orchestration boundary
- what is still deferred or not fully hardened

Use sections like:

```md
# Process Scheduling
## Current Runtime Model
## What Is Implemented Today
## Known Limits
## Source-of-Truth References
```

- [ ] **Step 2: Draft `docs/protocol/internal_bus.md` as a current-state bus contract note**

Summarize the current split and naming expectations around:
- task queue vs event bus semantics
- `tasks`
- `task.<id>`
- `task_results`
- audit and approval event topics

Reference the real source files and reset/backlog docs instead of inventing future protocol details.

- [ ] **Step 3: Draft `docs/journal/README.md` as a minimal, honest placeholder**

State:
- the purpose of the journal directory
- that historical entries may be added later
- where readers should look today for active source-of-truth decisions

- [ ] **Step 4: Verify the missing links now resolve**

Run:

```bash
test -f docs/architecture/process_scheduling.md
test -f docs/protocol/internal_bus.md
test -f docs/journal/README.md
```

Expected: all three commands succeed.

- [ ] **Step 5: Cross-check the new docs point back to real anchors**

Run: `rg -n "runtime-governance-reset|task_backlog|llm-execution-kernel|source-of-truth|Source-of-Truth" docs/architecture/process_scheduling.md docs/protocol/internal_bus.md docs/journal/README.md`
Expected: each document references at least one concrete source-of-truth path instead of standing alone as speculative prose.

- [ ] **Step 6: Commit the new anchor docs**

```bash
git add docs/architecture/process_scheduling.md docs/protocol/internal_bus.md docs/journal/README.md
git commit -m "docs: add runtime phase anchor docs"
```

### Task 3: Write the gold-path proof guide

**Files:**
- Create: `docs/testing/gold_path_proof.md`

- [ ] **Step 1: Create the proof document skeleton**

Structure the file around:

```md
# Gold-Path Proof Guide
## What This Proves
## Proof Buckets
## Exact Commands
## Coverage Boundaries
## What Is Still Deferred
```

- [ ] **Step 2: Map each proof bucket to exact existing tests**

Document these buckets and their current test anchors:
- `approval-success`
- `approval-reject`
- `approval-timeout-late-decision`
- `audit-replay`
- `sse-abort`
- `gateway-route`
- `smoke`
- `all`

Include the concrete tests from:
- `cmd/orchestrator/main_test.go`
- `internal/gateway/chat_completions_handler_test.go`
- `internal/gateway/sender_test.go`
- `internal/gateway/router_test.go`

- [ ] **Step 3: Write the exact `go test` commands that the script will dispatch**

Prefer stable package-scoped commands like:

```bash
go test ./internal/gateway -run 'Test(ChatCompletionsHandlerExecutesWriteToolAfterApproval|ChatCompletionsHandlerStreamsWriteToolAfterApproval)' -count=1
go test ./cmd/orchestrator -run 'Test(ServeHTTPCompletesWriteToolAfterApprovalWebhook|ServeHTTPLateApprovalWebhookDoesNotChangeTimedOutWriteToolState|SingleTaskReplayAuditPreservesTerminalResultStatus)' -count=1
go test ./internal/gateway -run 'Test(SenderDisconnectDoesNotEmitDoneFrame|SenderPublishesAbortedStreamAuditOnDisconnect|SenderPublishesDoneStatusAuditWhenFinalChunkDisconnects)' -count=1
go test ./internal/gateway -run 'Test(HandleIncomingTracksTaskRouteAndEnqueuesUnifiedPayload|StartStreamListenerRoutesRichMessageToRegisteredAdapter|StartStreamListenerDirectSendUsesExplicitChannelMessage|StartStreamListenerDirectSendOverridesRouteBinding)' -count=1
```

Adjust the final regexes so they match the exact current test names before implementing the script.

- [ ] **Step 4: State the proof boundaries explicitly**

Write down:
- what each bucket proves
- what each bucket does not prove
- that `smoke` must cross at least one real boundary and cannot just rename an in-memory unit test

- [ ] **Step 5: Verify the documented commands manually before scripting**

Run the exact commands you intend to place in the doc, one bucket at a time.
Expected: each command passes and matches the tests described in the document.

- [ ] **Step 6: Commit the proof guide**

```bash
git add docs/testing/gold_path_proof.md
git commit -m "docs: add gold-path proof guide"
```

### Task 4: Implement the named proof runner

**Files:**
- Create: `scripts/proof_gold_path.sh`

- [ ] **Step 1: Write the script skeleton with strict shell settings**

Start with:

```bash
#!/usr/bin/env bash
set -euo pipefail
```

Add:
- a usage block
- a bucket selector argument
- a `run_step` helper that prints readable section headers
- a `case` statement for each proof bucket

- [ ] **Step 2: Implement bucket dispatch using the exact commands from the proof doc**

Each bucket should run only its mapped commands. `all` should execute the buckets in a stable order:
1. `approval-success`
2. `approval-reject`
3. `approval-timeout-late-decision`
4. `audit-replay`
5. `sse-abort`
6. `gateway-route`
7. `smoke`

- [ ] **Step 3: Implement failure surfacing that is terminal- and LLM-friendly**

Print clear output like:

```bash
==> bucket: gateway-route
--> go test ./internal/gateway -run '...'
PASS: gateway-route
```

If a command fails:
- print the bucket name
- stop immediately with a non-zero exit
- do not continue into later buckets

- [ ] **Step 4: Add the minimal `smoke` bucket**

Pick one lightweight, current-phase-compatible smoke path that crosses a real boundary, for example:
- run a local handler path through an existing script or tiny `go test`/`go run` harness that uses real HTTP request handling, or
- run one real Redis-backed transport slice if a local Redis dependency is already available

Use this default unless there is a better equally small option in the repo:

```bash
go test ./cmd/orchestrator -run 'TestServeHTTPCompletesWriteToolAfterApprovalWebhook' -count=1
```

Reason: it exercises a real HTTP handler + approval webhook path and is therefore a better non-pure-fake proving slice than a transport-only unit test.

Keep the implementation narrow; the goal is one non-pure-fake proving slice, not a new integration framework.

- [ ] **Step 5: Verify script syntax and one focused bucket**

Run:

```bash
bash -n scripts/proof_gold_path.sh
bash scripts/proof_gold_path.sh gateway-route
```

Expected: syntax check passes; the focused bucket prints readable output and exits `0`.

- [ ] **Step 6: Commit the proof runner**

```bash
git add scripts/proof_gold_path.sh
git commit -m "chore: add gold-path proof runner"
```

### Task 5: Final cross-check and execution handoff

**Files:**
- Modify: `README.md`
- Modify: `docs/README.md`
- Create: `docs/testing/gold_path_proof.md`
- Create: `docs/architecture/process_scheduling.md`
- Create: `docs/protocol/internal_bus.md`
- Create: `docs/journal/README.md`
- Create: `scripts/proof_gold_path.sh`

- [ ] **Step 1: Run the full proof suite**

Run: `bash scripts/proof_gold_path.sh all`
Expected: all named buckets pass in order, including the final smoke slice.

- [ ] **Step 2: Re-run the broad Go baseline after documentation/script changes**

Run: `GOCACHE="$(pwd)/.cache/go-build" go test ./... -timeout 30s`
Expected: PASS. If the local environment again requires leaving the sandbox for listener tests, use the same approved outside-sandbox rerun and record the reason.

- [ ] **Step 3: Verify the docs hub points to every new artifact**

Run:

```bash
rg -n "gold_path_proof|process_scheduling|internal_bus|journal/README" README.md docs/README.md
```

Expected: documentation entry points reference the new files.

- [ ] **Step 4: Review the final diff for scope control**

Run: `git diff --stat docs/phase-a-specs..HEAD`
Expected: only Phase A docs and proof-runner files appear; no unrelated runtime or gateway logic changes.

- [ ] **Step 5: Commit only if the final validation required follow-up edits**

```bash
git add README.md docs/README.md docs/testing/gold_path_proof.md docs/architecture/process_scheduling.md docs/protocol/internal_bus.md docs/journal/README.md scripts/proof_gold_path.sh
git commit -m "docs: expose phase a proof and runtime truth"
```

If Step 1-4 passed without further edits, skip this commit instead of creating an empty commit.

- [ ] **Step 6: Prepare the branch for execution review**

Run:

```bash
git status --short --branch
git log --oneline --decorate -n 5
```

Expected: clean branch with a short, readable commit stack ready for execution review or PR preparation.
