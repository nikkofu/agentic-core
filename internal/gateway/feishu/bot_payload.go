package feishu

import (
	"agentic-core/internal/gateway"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strconv"
	"time"
)

func buildBotPayload(msg gateway.ChannelResponse, now time.Time, secret string) (map[string]any, error) {
	payload := map[string]any{}

	if secret != "" {
		timestamp := strconv.FormatInt(now.Unix(), 10)
		payload["timestamp"] = timestamp
		payload["sign"] = botSign(timestamp, secret)
	}

	switch {
	case msg.Card != nil:
		payload["msg_type"] = "interactive"
		payload["card"] = msg.Card
	case msg.MessageType == gateway.MessageTypeMarkdown || msg.Format == gateway.FormatMarkdown:
		payload["msg_type"] = "post"
		payload["content"] = map[string]any{
			"post": map[string]any{
				"zh_cn": map[string]any{
					"title": "",
					"content": []any{
						[]any{
							map[string]any{
								"tag":  "text",
								"text": msg.Text,
							},
						},
					},
				},
			},
		}
	default:
		payload["msg_type"] = "text"
		payload["content"] = map[string]any{
			"text": msg.Text,
		}
	}

	return payload, nil
}

func botSign(timestamp, secret string) string {
	key := []byte(fmt.Sprintf("%s\n%s", timestamp, secret))
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte{})
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}
