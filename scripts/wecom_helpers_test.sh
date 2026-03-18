#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"

failures=0

pass() {
  printf 'PASS: %s\n' "$1"
}

fail() {
  printf 'FAIL: %s\n' "$1"
  failures=$((failures + 1))
}

test_signed_ingress_script() {
  local script="$ROOT_DIR/scripts/wecom_signed_ingress.sh"
  if [ ! -x "$script" ]; then
    fail "signed ingress helper exists and is executable"
    return
  fi

  local tmp
  tmp="$(mktemp -d)"
  trap 'rm -rf "${tmp:-}"' RETURN

  local payload="$tmp/payload.json"
  cat >"$payload" <<'JSON'
{"session_id":"robot-demo-1","channel_name":"wecom_robot","message_type":"markdown","format":"markdown","text":"**deploy ok**"}
JSON

  local args_file="$tmp/args.txt"
  local body_file="$tmp/body.json"
  cat >"$tmp/curl" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$@" > "$TMP_ARGS_FILE"
prev=""
for arg in "$@"; do
  if [ "$prev" = "--data-binary" ]; then
    if [[ "$arg" == @* ]]; then
      cat "${arg#@}" > "$TMP_BODY_FILE"
    else
      printf '%s' "$arg" > "$TMP_BODY_FILE"
    fi
  fi
  prev="$arg"
done
printf '{"status":"accepted"}\n'
EOF
  chmod +x "$tmp/curl"

  local ts="1773844800"
  local nonce="nonce-robot-001"
  local expected_sig
  expected_sig="$(printf "%s.%s.%s" "$ts" "$nonce" "$(cat "$payload")" | openssl dgst -sha256 -hmac "secret-123" -binary | xxd -p -c 256)"

  TMP_ARGS_FILE="$args_file" \
  TMP_BODY_FILE="$body_file" \
  PATH="$tmp:$PATH" \
  GATEWAY_BASE_URL="http://127.0.0.1:8081" \
  GATEWAY_INGRESS_SECRET="secret-123" \
  AGENTIC_TIMESTAMP="$ts" \
  AGENTIC_NONCE="$nonce" \
  "$script" "$payload" >/dev/null

  if grep -Fq "http://127.0.0.1:8081/v1/channels/incoming" "$args_file"; then
    pass "signed ingress helper targets unified ingress endpoint"
  else
    fail "signed ingress helper targets unified ingress endpoint"
  fi

  if grep -Fq "X-Agentic-Signature: $expected_sig" "$args_file"; then
    pass "signed ingress helper emits expected signature header"
  else
    fail "signed ingress helper emits expected signature header"
  fi

  if cmp -s "$payload" "$body_file"; then
    pass "signed ingress helper sends payload file contents"
  else
    fail "signed ingress helper sends payload file contents"
  fi
}

test_robot_markdown_demo() {
  local script="$ROOT_DIR/scripts/wecom_robot_markdown_demo.sh"
  if [ ! -x "$script" ]; then
    fail "robot markdown demo helper exists and is executable"
    return
  fi

  local tmp
  tmp="$(mktemp -d)"
  trap 'rm -rf "${tmp:-}"' RETURN

  local capture="$tmp/payload.json"
  cat >"$tmp/ingress-helper" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
cp "$1" "$CAPTURE_FILE"
printf '{"status":"accepted"}\n'
EOF
  chmod +x "$tmp/ingress-helper"

  CAPTURE_FILE="$capture" \
  WECOM_SIGNED_INGRESS_SCRIPT="$tmp/ingress-helper" \
  WECOM_ROBOT_WEBHOOK_URL="https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=robot-key" \
  ROBOT_TEXT="**robot ok**" \
  "$script" >/dev/null

  if grep -Fq '"channel_name":"wecom_robot"' "$capture"; then
    pass "robot markdown demo targets wecom_robot channel"
  else
    fail "robot markdown demo targets wecom_robot channel"
  fi

  if grep -Fq '"message_type":"markdown"' "$capture"; then
    pass "robot markdown demo uses markdown type"
  else
    fail "robot markdown demo uses markdown type"
  fi

  if grep -Fq '**robot ok**' "$capture" && grep -Fq 'robot-key' "$capture"; then
    pass "robot markdown demo embeds text and webhook url"
  else
    fail "robot markdown demo embeds text and webhook url"
  fi
}

test_callback_demo_helper() {
  local script="$ROOT_DIR/scripts/wecom_callback_demo.sh"
  if [ ! -x "$script" ]; then
    fail "callback demo helper exists and is executable"
    return
  fi

  local tmp
  tmp="$(mktemp -d)"
  trap 'rm -rf "${tmp:-}"' RETURN

  local args_file="$tmp/args.txt"
  local xml_capture="$tmp/inner.xml"
  cat >"$tmp/simulator" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$@" > "$SIM_ARGS_FILE"
prev=""
for arg in "$@"; do
  if [ "$prev" = "-inner-xml" ] || [ "$prev" = "--inner-xml" ]; then
    cp "$arg" "$SIM_XML_FILE"
  fi
  prev="$arg"
done
printf '{"status":"ok"}\n'
EOF
  chmod +x "$tmp/simulator"

  WECOM_CALLBACK_SIM_BIN="$tmp/simulator" \
  SIM_ARGS_FILE="$args_file" \
  SIM_XML_FILE="$xml_capture" \
  WECOM_CORP_ID="ww-test-corp" \
  WECOM_AGENT_ID="1000002" \
  WECOM_TOKEN="gateway-token" \
  WECOM_ENCODING_AES_KEY="abcdefghijklmnopqrstuvwxyz0123456789ABCDEFG" \
  WECOM_CALLBACK_TIMESTAMP="1773811200" \
  WECOM_CALLBACK_NONCE="nonce-callback-001" \
  WECOM_CALLBACK_FROM_USER="lisi" \
  WECOM_CALLBACK_CONTENT="hello callback" \
  "$script" >/dev/null

  if grep -Fq "http://127.0.0.1:8081/callbacks/wecom" "$args_file"; then
    pass "callback demo helper passes default callback endpoint"
  else
    fail "callback demo helper passes default callback endpoint"
  fi

  if grep -Fq "ww-test-corp" "$args_file" && grep -Fq "gateway-token" "$args_file"; then
    pass "callback demo helper forwards WeCom credentials"
  else
    fail "callback demo helper forwards WeCom credentials"
  fi

  if grep -Fq "1773811200" "$args_file" && grep -Fq "nonce-callback-001" "$args_file"; then
    pass "callback demo helper forwards timestamp and nonce overrides"
  else
    fail "callback demo helper forwards timestamp and nonce overrides"
  fi

  if grep -Fq "<FromUserName><![CDATA[lisi]]>" "$xml_capture" && grep -Fq "<Content><![CDATA[hello callback]]>" "$xml_capture"; then
    pass "callback demo helper builds default text inner xml"
  else
    fail "callback demo helper builds default text inner xml"
  fi
}

test_callback_template_fixtures() {
  local base="$ROOT_DIR/scripts/wecom_callback_templates"
  for name in image voice video file link location event; do
    if [ -f "$base/$name.xml" ]; then
      pass "callback template exists: $name"
    else
      fail "callback template exists: $name"
    fi
  done
}

assert_template_render() {
  local template_name="$1"
  local expected_a="$2"
  local expected_b="$3"
  local script="$ROOT_DIR/scripts/wecom_callback_demo.sh"
  local tmp
  tmp="$(mktemp -d)"
  trap 'rm -rf "${tmp:-}"' RETURN

  local args_file="$tmp/args.txt"
  local xml_capture="$tmp/inner.xml"
  cat >"$tmp/simulator" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$@" > "$SIM_ARGS_FILE"
prev=""
for arg in "$@"; do
  if [ "$prev" = "-inner-xml" ]; then
    cp "$arg" "$SIM_XML_FILE"
  fi
  prev="$arg"
done
printf '{"status":"ok"}\n'
EOF
  chmod +x "$tmp/simulator"

  WECOM_CALLBACK_SIM_BIN="$tmp/simulator" \
  SIM_ARGS_FILE="$args_file" \
  SIM_XML_FILE="$xml_capture" \
  WECOM_CORP_ID="ww-test-corp" \
  WECOM_AGENT_ID="1000002" \
  WECOM_TOKEN="gateway-token" \
  WECOM_ENCODING_AES_KEY="abcdefghijklmnopqrstuvwxyz0123456789ABCDEFG" \
  WECOM_CALLBACK_TEMPLATE="$template_name" \
  WECOM_CALLBACK_FROM_USER="wangwu" \
  WECOM_CALLBACK_MEDIA_ID="MEDIA_TEST_001" \
  WECOM_CALLBACK_THUMB_MEDIA_ID="MEDIA_THUMB_001" \
  WECOM_CALLBACK_PIC_URL="https://example.com/demo.jpg" \
  WECOM_CALLBACK_FORMAT="amr" \
  WECOM_CALLBACK_TITLE="报警详情" \
  WECOM_CALLBACK_DESCRIPTION="主机 CPU 超过阈值" \
  WECOM_CALLBACK_URL="https://example.com/ticket/42" \
  WECOM_CALLBACK_LOCATION_X="31.2304" \
  WECOM_CALLBACK_LOCATION_Y="121.4737" \
  WECOM_CALLBACK_SCALE="15" \
  WECOM_CALLBACK_LABEL="上海市黄浦区" \
  WECOM_CALLBACK_EVENT="change_contact" \
  WECOM_CALLBACK_CHANGE_TYPE="create_user" \
  "$script" >/dev/null

  if grep -Fq "$expected_a" "$xml_capture" && grep -Fq "$expected_b" "$xml_capture"; then
    pass "callback template renders: $template_name"
  else
    fail "callback template renders: $template_name"
  fi
}

test_callback_template_rendering() {
  assert_template_render "image" "<MsgType><![CDATA[image]]>" "<MediaId><![CDATA[MEDIA_TEST_001]]>"
  assert_template_render "voice" "<MsgType><![CDATA[voice]]>" "<Format><![CDATA[amr]]>"
  assert_template_render "video" "<MsgType><![CDATA[video]]>" "<ThumbMediaId><![CDATA[MEDIA_THUMB_001]]>"
  assert_template_render "file" "<MsgType><![CDATA[file]]>" "<MediaId><![CDATA[MEDIA_TEST_001]]>"
  assert_template_render "link" "<MsgType><![CDATA[link]]>" "<Url><![CDATA[https://example.com/ticket/42]]>"
  assert_template_render "location" "<MsgType><![CDATA[location]]>" "<Label><![CDATA[上海市黄浦区]]>"
  assert_template_render "event" "<MsgType><![CDATA[event]]>" "<ChangeType><![CDATA[create_user]]>"
}

test_callback_template_selection_priority() {
  local script="$ROOT_DIR/scripts/wecom_callback_demo.sh"
  local tmp
  tmp="$(mktemp -d)"
  trap 'rm -rf "${tmp:-}"' RETURN

  local xml_capture="$tmp/inner.xml"
  cat >"$tmp/simulator" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
prev=""
for arg in "$@"; do
  if [ "$prev" = "-inner-xml" ]; then
    cp "$arg" "$SIM_XML_FILE"
  fi
  prev="$arg"
done
printf '{"status":"ok"}\n'
EOF
  chmod +x "$tmp/simulator"

  WECOM_CALLBACK_SIM_BIN="$tmp/simulator" \
  SIM_XML_FILE="$xml_capture" \
  WECOM_CORP_ID="ww-test-corp" \
  WECOM_AGENT_ID="1000002" \
  WECOM_TOKEN="gateway-token" \
  WECOM_ENCODING_AES_KEY="abcdefghijklmnopqrstuvwxyz0123456789ABCDEFG" \
  WECOM_CALLBACK_TEMPLATE="image" \
  WECOM_CALLBACK_THUMB_MEDIA_ID="MEDIA_THUMB_009" \
  "$script" video >/dev/null

  if grep -Fq "<MsgType><![CDATA[video]]>" "$xml_capture"; then
    pass "callback demo positional template name overrides template env"
  else
    fail "callback demo positional template name overrides template env"
  fi
}

printf 'Running WeCom helper script checks in %s\n' "$ROOT_DIR"

test_signed_ingress_script
test_robot_markdown_demo
test_callback_demo_helper
test_callback_template_fixtures
test_callback_template_rendering
test_callback_template_selection_priority

if [ "$failures" -gt 0 ]; then
  printf '\nWeCom helper tests FAILED with %d issue(s).\n' "$failures"
  exit 1
fi

printf '\nWeCom helper tests PASSED.\n'
