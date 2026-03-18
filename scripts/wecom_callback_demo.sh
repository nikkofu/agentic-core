#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
TEMPLATE_DIR="$ROOT_DIR/scripts/wecom_callback_templates"

require_env() {
  local key="$1"
  if [ "${!key:-}" = "" ]; then
    printf '%s is required\n' "$key" >&2
    exit 1
  fi
}

require_env "WECOM_CORP_ID"
require_env "WECOM_AGENT_ID"
require_env "WECOM_TOKEN"
require_env "WECOM_ENCODING_AES_KEY"

render_template() {
  local template_file="$1"
  local content
  content="$(cat "$template_file")"
  content="${content//__CORP_ID__/$WECOM_CORP_ID}"
  content="${content//__FROM_USER__/${WECOM_CALLBACK_FROM_USER:-zhangsan}}"
  content="${content//__CREATE_TIME__/${WECOM_CALLBACK_CREATE_TIME:-$(date +%s)}}"
  content="${content//__MSG_ID__/${WECOM_CALLBACK_MSG_ID:-1001}}"
  content="${content//__AGENT_ID__/$WECOM_AGENT_ID}"
  content="${content//__MEDIA_ID__/${WECOM_CALLBACK_MEDIA_ID:-MEDIA_TEST_001}}"
  content="${content//__THUMB_MEDIA_ID__/${WECOM_CALLBACK_THUMB_MEDIA_ID:-MEDIA_THUMB_001}}"
  content="${content//__PIC_URL__/${WECOM_CALLBACK_PIC_URL:-https://example.com/demo.jpg}}"
  content="${content//__FORMAT__/${WECOM_CALLBACK_FORMAT:-amr}}"
  content="${content//__TITLE__/${WECOM_CALLBACK_TITLE:-告警详情}}"
  content="${content//__DESCRIPTION__/${WECOM_CALLBACK_DESCRIPTION:-企业微信本地回调联调}}"
  content="${content//__URL__/${WECOM_CALLBACK_URL:-https://example.com/ticket/42}}"
  content="${content//__LOCATION_X__/${WECOM_CALLBACK_LOCATION_X:-31.2304}}"
  content="${content//__LOCATION_Y__/${WECOM_CALLBACK_LOCATION_Y:-121.4737}}"
  content="${content//__SCALE__/${WECOM_CALLBACK_SCALE:-15}}"
  content="${content//__LABEL__/${WECOM_CALLBACK_LABEL:-上海市黄浦区}}"
  content="${content//__EVENT__/${WECOM_CALLBACK_EVENT:-change_contact}}"
  content="${content//__CHANGE_TYPE__/${WECOM_CALLBACK_CHANGE_TYPE:-create_user}}"
  printf '%s' "$content"
}

endpoint="${WECOM_CALLBACK_ENDPOINT:-${GATEWAY_BASE_URL:-http://127.0.0.1:8081}/callbacks/wecom}"

inner_xml_path="${1:-}"
tmp_inner_xml=""
if [ "$inner_xml_path" != "" ] && [ ! -f "$inner_xml_path" ] && [ -f "$TEMPLATE_DIR/$inner_xml_path.xml" ]; then
  tmp_inner_xml="$(mktemp)"
  trap 'rm -f "${tmp_inner_xml:-}"' EXIT
  render_template "$TEMPLATE_DIR/$inner_xml_path.xml" >"$tmp_inner_xml"
  inner_xml_path="$tmp_inner_xml"
elif [ "$inner_xml_path" = "" ] && [ "${WECOM_CALLBACK_TEMPLATE:-}" != "" ]; then
  if [ ! -f "$TEMPLATE_DIR/$WECOM_CALLBACK_TEMPLATE.xml" ]; then
    printf 'callback template not found: %s\n' "$WECOM_CALLBACK_TEMPLATE" >&2
    exit 1
  fi
  tmp_inner_xml="$(mktemp)"
  trap 'rm -f "${tmp_inner_xml:-}"' EXIT
  render_template "$TEMPLATE_DIR/$WECOM_CALLBACK_TEMPLATE.xml" >"$tmp_inner_xml"
  inner_xml_path="$tmp_inner_xml"
elif [ "$inner_xml_path" = "" ]; then
  tmp_inner_xml="$(mktemp)"
  trap 'rm -f "${tmp_inner_xml:-}"' EXIT
  create_time="${WECOM_CALLBACK_CREATE_TIME:-$(date +%s)}"
  from_user="${WECOM_CALLBACK_FROM_USER:-zhangsan}"
  content="${WECOM_CALLBACK_CONTENT:-hello gateway}"
  msg_id="${WECOM_CALLBACK_MSG_ID:-1001}"
  cat >"$tmp_inner_xml" <<EOF
<xml>
<ToUserName><![CDATA[$WECOM_CORP_ID]]></ToUserName>
<FromUserName><![CDATA[$from_user]]></FromUserName>
<CreateTime>$create_time</CreateTime>
<MsgType><![CDATA[text]]></MsgType>
<Content><![CDATA[$content]]></Content>
<MsgId>$msg_id</MsgId>
<AgentID>$WECOM_AGENT_ID</AgentID>
</xml>
EOF
  inner_xml_path="$tmp_inner_xml"
fi

if [ ! -f "$inner_xml_path" ]; then
  printf 'inner xml file not found: %s\n' "$inner_xml_path" >&2
  exit 1
fi

if [ "${WECOM_CALLBACK_SIM_BIN:-}" != "" ]; then
  "$WECOM_CALLBACK_SIM_BIN" \
    -endpoint "$endpoint" \
    -corp-id "$WECOM_CORP_ID" \
    -agent-id "$WECOM_AGENT_ID" \
    -token "$WECOM_TOKEN" \
    -encoding-aes-key "$WECOM_ENCODING_AES_KEY" \
    -inner-xml "$inner_xml_path" \
    ${WECOM_CALLBACK_TIMESTAMP:+-timestamp "$WECOM_CALLBACK_TIMESTAMP"} \
    ${WECOM_CALLBACK_NONCE:+-nonce "$WECOM_CALLBACK_NONCE"}
else
  (
    cd "$ROOT_DIR"
    go run ./cmd/wecom-callback-sim \
      -endpoint "$endpoint" \
      -corp-id "$WECOM_CORP_ID" \
      -agent-id "$WECOM_AGENT_ID" \
      -token "$WECOM_TOKEN" \
      -encoding-aes-key "$WECOM_ENCODING_AES_KEY" \
      -inner-xml "$inner_xml_path" \
      ${WECOM_CALLBACK_TIMESTAMP:+-timestamp "$WECOM_CALLBACK_TIMESTAMP"} \
      ${WECOM_CALLBACK_NONCE:+-nonce "$WECOM_CALLBACK_NONCE"}
  )
fi
