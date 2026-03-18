#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"

json_escape() {
  local value="${1:-}"
  value="${value//\\/\\\\}"
  value="${value//\"/\\\"}"
  value="${value//$'\n'/\\n}"
  value="${value//$'\r'/\\r}"
  value="${value//$'\t'/\\t}"
  printf '%s' "$value"
}

helper_script="${WECOM_SIGNED_INGRESS_SCRIPT:-$ROOT_DIR/scripts/wecom_signed_ingress.sh}"
if [ ! -x "$helper_script" ]; then
  printf 'signed ingress helper is not executable: %s\n' "$helper_script" >&2
  exit 1
fi

webhook_url="${WECOM_ROBOT_WEBHOOK_URL:-}"
if [ "$webhook_url" = "" ]; then
  printf 'WECOM_ROBOT_WEBHOOK_URL is required\n' >&2
  exit 1
fi

session_id="${ROBOT_SESSION_ID:-robot-demo-1}"
text="${ROBOT_TEXT:-**deploy success**}"

tmp_payload="$(mktemp)"
trap 'rm -f "$tmp_payload"' EXIT

cat >"$tmp_payload" <<EOF
{"session_id":"$(json_escape "$session_id")","channel_name":"wecom_robot","message_type":"markdown","format":"markdown","text":"$(json_escape "$text")","metadata":{"wecom_robot_webhook_url":"$(json_escape "$webhook_url")"}}
EOF

"$helper_script" "$tmp_payload"
