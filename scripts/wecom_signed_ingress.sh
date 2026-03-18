#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage: scripts/wecom_signed_ingress.sh <payload.json>

Environment:
  GATEWAY_BASE_URL        Optional, defaults to http://127.0.0.1:8081
  GATEWAY_INGRESS_URL     Optional, overrides the full ingress endpoint URL
  GATEWAY_INGRESS_SECRET  Optional, when set emits X-Agentic-* signature headers
  AGENTIC_TIMESTAMP       Optional, overrides the signature timestamp
  AGENTIC_NONCE           Optional, overrides the signature nonce
EOF
}

if [ "${1:-}" = "" ]; then
  usage >&2
  exit 1
fi

payload_file="$1"
if [ ! -f "$payload_file" ]; then
  printf 'payload file not found: %s\n' "$payload_file" >&2
  exit 1
fi

base_url="${GATEWAY_BASE_URL:-http://127.0.0.1:8081}"
ingress_url="${GATEWAY_INGRESS_URL:-${base_url%/}/v1/channels/incoming}"

curl_args=(
  --silent
  --show-error
  -X POST
  "$ingress_url"
  -H "Content-Type: application/json"
  --data-binary "@$payload_file"
)

if [ "${GATEWAY_INGRESS_SECRET:-}" != "" ]; then
  timestamp="${AGENTIC_TIMESTAMP:-$(date +%s)}"
  nonce="${AGENTIC_NONCE:-nonce-${timestamp}}"
  body="$(cat "$payload_file")"
  signature="$(printf "%s.%s.%s" "$timestamp" "$nonce" "$body" | openssl dgst -sha256 -hmac "$GATEWAY_INGRESS_SECRET" -binary | xxd -p -c 256)"
  curl_args+=(
    -H "X-Agentic-Timestamp: $timestamp"
    -H "X-Agentic-Nonce: $nonce"
    -H "X-Agentic-Signature: $signature"
  )
fi

curl "${curl_args[@]}"
