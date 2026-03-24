# Leftover PR Convergence (PR#5 → PR#4) Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Produce explicit, evidence-backed final decisions for PR#5 and PR#4 (merge / close as superseded / continue patching) without introducing unrelated work.

**Architecture:** Execute sequentially: first PR#5 (`pr/task01-strict-compliance`), then PR#4 (`plan/phase-a-doc-proof`). For each PR, run the same pipeline: diff audit → targeted verification → minimal patch if needed (RED→GREEN) → decision output with command-level evidence.

**Tech Stack:** Git/GitHub CLI (`gh`), Go test tooling, repository docs/proof script.

---

## File Map

- Verify / possibly modify on PR#5 branch only:
  - `internal/process/manager.go`
  - `internal/process/exec_manager.go`
  - `internal/process/exec_manager_test.go`
  - `cmd/orchestrator/main.go`
  - `cmd/orchestrator/main_test.go`
  - `docker-compose.yml`
- Verify / possibly modify on PR#4 branch only:
  - `README.md`
  - `docs/README.md`
  - `docs/architecture/process_scheduling.md`
  - `docs/protocol/internal_bus.md`
  - `docs/journal/README.md`
  - `docs/testing/gold_path_proof.md`
  - `scripts/proof_gold_path.sh`
  - `cmd/orchestrator/main_test.go`
- Evidence report artifact (if repo has no fixed report file, keep in session summary + PR comments):
  - PR decision record for #5 and #4 with commands + outputs.

---

### Task 0: Baseline and PR inventory lock

**Files:**
- Verify only (no code changes)

- [ ] **Step 1: Capture local baseline state**

Run: `git status --short --branch`
Expected: clear view of local dirty state before convergence actions.

- [ ] **Step 2: Capture open PR inventory**

Run: `gh pr list --state open --limit 20`
Expected: PR#5 and PR#4 visible with head branches.

- [ ] **Step 3: Confirm branch names and PR metadata**

Run:
- `gh pr view 5 --json number,title,headRefName,baseRefName,url`
- `gh pr view 4 --json number,title,headRefName,baseRefName,url`

Expected: deterministic branch targets for downstream steps.

- [ ] **Step 4: Commit baseline note (optional, docs-only)**

If a tracking note file is used in your workflow, commit only that note; otherwise skip commit.

---

### Task 1: Converge PR#5 (`pr/task01-strict-compliance`)

**Files:**
- Verify/modify:
  - `internal/process/manager.go`
  - `internal/process/exec_manager.go`
  - `internal/process/exec_manager_test.go`
  - `cmd/orchestrator/main.go`
  - `cmd/orchestrator/main_test.go`
  - `docker-compose.yml`

- [ ] **Step 1: Checkout PR#5 branch and sync baseline**

Run:
- `git fetch --all --prune`
- `git checkout pr/task01-strict-compliance`
- `git pull --ff-only`

Expected: clean, up-to-date PR#5 branch.

- [ ] **Step 2: Run diff audit against main**

Run:
- `gh pr view 5 --json commits,files,headRefName,baseRefName,url`
- `git diff --stat main...pr/task01-strict-compliance`
- `git diff --name-only main...pr/task01-strict-compliance`

Expected: file-level delta set for coverage classification.

- [ ] **Step 3: Produce coverage classification (covered / partial / uncovered)**

Using step-2 diff and current `main`, write explicit classification bullets in session notes.

Expected: no ambiguous items; every PR#5 file/change is classified.

- [ ] **Step 4: Run focused regression on PR#5 scope**

Run:
- `go test ./internal/process ./cmd/orchestrator -count=1`

Expected: PASS, or concrete failing tests identifying patch target.

- [ ] **Step 5: Validate compose default-port compliance**

Run (one of):
- project’s existing duplicate-port checker, or
- manual deterministic check of `docker-compose.yml` host default ports.

Expected: no duplicate default host ports.

- [ ] **Step 6: If needed, write failing test/check first (RED)**

If step 4/5 exposes a gap, add minimal failing test or deterministic check tied to that exact gap.

Expected: RED reproduces the gap.

- [ ] **Step 7: Implement minimal branch-local fix (GREEN)**

Patch only PR#5-scope files. No unrelated refactor.

Expected: gap resolved with smallest possible diff.

- [ ] **Step 8: Re-run focused verification**

Run:
- `go test ./internal/process ./cmd/orchestrator -count=1`
- any gap-specific check from step 6.

Expected: GREEN.

- [ ] **Step 9: Commit PR#5 patch only if modifications were required**

```bash
git add <only-pr5-scoped-files>
git commit -m "fix(pr5): close strict-compliance convergence gap"
```

- [ ] **Step 10: Make explicit PR#5 decision with evidence**

Decision must be one of:
- Merge
- Close as superseded
- Continue patching

Include commands run + key outputs that justify decision.

---

### Task 2: Converge PR#4 (`plan/phase-a-doc-proof`)

**Files:**
- Verify/modify:
  - `README.md`
  - `docs/README.md`
  - `docs/architecture/process_scheduling.md`
  - `docs/protocol/internal_bus.md`
  - `docs/journal/README.md`
  - `docs/testing/gold_path_proof.md`
  - `scripts/proof_gold_path.sh`
  - `cmd/orchestrator/main_test.go`

- [ ] **Step 1: Checkout PR#4 branch and sync baseline**

Run:
- `git checkout plan/phase-a-doc-proof`
- `git pull --ff-only`

Expected: clean, up-to-date PR#4 branch.

- [ ] **Step 2: Run diff audit against main**

Run:
- `gh pr view 4 --json commits,files,headRefName,baseRefName,url`
- `git diff --stat main...plan/phase-a-doc-proof`
- `git diff --name-only main...plan/phase-a-doc-proof`

Expected: clear doc/proof delta set.

- [ ] **Step 3: Produce coverage classification (covered / partial / uncovered)**

Classify each changed doc/proof artifact against current `main` truth.

Expected: explicit coverage matrix for PR#4 changes.

- [ ] **Step 4: Run link/truth-alignment verification**

Run link checks or deterministic manual checks for changed docs.

Expected: no broken links; statements align with current implementation state.

- [ ] **Step 5: Run proof entry verification**

Run: `bash scripts/proof_gold_path.sh`
Expected: script executes and returns actionable proof output.

- [ ] **Step 6: If needed, write failing check first (RED)**

If docs/proof mismatch is found, add minimal check/update target showing mismatch.

Expected: RED demonstrates the exact gap.

- [ ] **Step 7: Apply minimal PR#4-scope patch (GREEN)**

Patch only changed docs/proof script/tests required by identified gaps.

Expected: truth-aligned and verifiable docs/proof behavior.

- [ ] **Step 8: Re-run PR#4 verification**

Re-run step 4/5 commands.

Expected: GREEN.

- [ ] **Step 9: Commit PR#4 patch only if modifications were required**

```bash
git add <only-pr4-scoped-files>
git commit -m "docs(pr4): align proof docs with mainline truth"
```

- [ ] **Step 10: Make explicit PR#4 decision with evidence**

Decision must be one of:
- Merge
- Close as superseded
- Continue patching

Include commands run + key outputs.

---

### Task 3: Final convergence verification and report

**Files:**
- Verify repo/PR state only

- [ ] **Step 1: Return to main and run final regression gate**

Run:
- `git checkout main`
- `git pull --ff-only`
- `go test ./... -count=1`

Expected: PASS.

- [ ] **Step 2: Verify no scope creep**

Run:
- `git diff --name-only main...pr/task01-strict-compliance`
- `git diff --name-only main...plan/phase-a-doc-proof`

Expected: changes remain within each PR’s intended scope.

- [ ] **Step 3: Produce final convergence report**

For PR#5 and PR#4 include:
- final decision,
- evidence commands and outputs,
- whether patching occurred,
- residual risk and exact next action (if continue patching).

- [ ] **Step 4: Optional final commit (report file only)**

If report is saved to repo docs, commit report-only change.

---

## Acceptance Matrix

- [ ] PR#5 has explicit, evidenced final decision.
- [ ] PR#4 has explicit, evidenced final decision.
- [ ] Any required patching happened on original PR branches only.
- [ ] Convergence actions introduced no unrelated feature work.
- [ ] Mainline critical tests pass (`go test ./... -count=1`).
- [ ] Final report includes command-level evidence for both PR outcomes.
