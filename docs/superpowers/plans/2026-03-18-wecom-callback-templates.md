# WeCom Callback Templates Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add reusable local callback fixtures and demo flows for Enterprise WeChat image, voice, video, file, link, location, and event callbacks so more inbound message types can be validated without the WeCom admin console.

**Architecture:** Keep the existing callback simulator as the transport layer and add a small set of XML template fixtures plus template rendering in `scripts/wecom_callback_demo.sh`. Templates will fill only the fields already parsed by `internal/gateway/wecom/handler.go` so local demos stay aligned with the unified gateway receive path.

**Tech Stack:** Bash, static XML fixtures, existing `cmd/wecom-callback-sim`, existing shell test harness.

---

### Task 1: Add failing template fixture tests

**Files:**
- Modify: `scripts/wecom_helpers_test.sh`

- [ ] **Step 1: Write the failing shell test**

Add checks that:
- template XML files exist for `image`, `voice`, `video`, `file`, `link`, `location`, `event`
- `scripts/wecom_callback_demo.sh` can render and pass each template to the simulator

- [ ] **Step 2: Run test to verify it fails**

Run: `bash scripts/wecom_helpers_test.sh`
Expected: FAIL because the template fixtures and template rendering flow do not exist yet.

### Task 2: Add callback template fixtures

**Files:**
- Create: `scripts/wecom_callback_templates/image.xml`
- Create: `scripts/wecom_callback_templates/voice.xml`
- Create: `scripts/wecom_callback_templates/video.xml`
- Create: `scripts/wecom_callback_templates/file.xml`
- Create: `scripts/wecom_callback_templates/link.xml`
- Create: `scripts/wecom_callback_templates/location.xml`
- Create: `scripts/wecom_callback_templates/event.xml`

- [ ] **Step 1: Write minimal fixture files**

Each fixture should use environment-style placeholders for the small set of values the demo script fills.

- [ ] **Step 2: Run shell test to check fixture presence**

Run: `bash scripts/wecom_helpers_test.sh`
Expected: partial progress or remaining FAIL only for demo rendering.

### Task 3: Wire template rendering into demo script

**Files:**
- Modify: `scripts/wecom_callback_demo.sh`

- [ ] **Step 1: Implement template selection**

Support `WECOM_CALLBACK_TEMPLATE=image|voice|video|file|link|location|event` and render the matching template when no explicit inner XML file is passed.

- [ ] **Step 2: Implement minimal placeholder replacement**

Support common fields such as `corp_id`, `from_user`, `create_time`, `msg_id`, `agent_id`, `media_id`, `thumb_media_id`, `pic_url`, `format`, `title`, `description`, `url`, `location_x`, `location_y`, `scale`, `label`, `event`, and `change_type`.

- [ ] **Step 3: Run shell test to verify it passes**

Run: `bash scripts/wecom_helpers_test.sh`
Expected: PASS

### Task 4: Document template usage

**Files:**
- Modify: `docs/gateway_wecom_runbook.md`

- [ ] **Step 1: Add template examples**

Document example commands for `image`, `voice`, `video`, `file`, `link`, `location`, and `event` callback simulation.

- [ ] **Step 2: Run focused verification**

Run: `bash scripts/wecom_helpers_test.sh && go test ./internal/gateway ./internal/gateway/wecom ./internal/gateway/wecomrobot ./cmd/gateway ./cmd/wecom-callback-sim -v`
Expected: PASS
