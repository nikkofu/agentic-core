package dingtalk

import (
	"agentic-core/internal/gateway"
	"fmt"
)

func buildRobotPayload(msg gateway.ChannelResponse) (map[string]any, error) {
	if len(msg.Card) > 0 {
		payload := make(map[string]any, len(msg.Card))
		for key, value := range msg.Card {
			payload[key] = value
		}
		if _, ok := payload["msgtype"]; !ok {
			return nil, fmt.Errorf("dingtalk robot card payload requires msgtype")
		}
		return payload, nil
	}

	msgType := effectiveRobotMessageType(msg)
	switch msgType {
	case "text":
		return map[string]any{
			"msgtype": "text",
			"text": map[string]any{
				"content": msg.Text,
			},
		}, nil
	case "markdown":
		title := "Agentic-Core"
		if value, ok := msg.Metadata["title"]; ok && fmt.Sprint(value) != "" {
			title = fmt.Sprint(value)
		}
		return map[string]any{
			"msgtype": "markdown",
			"markdown": map[string]any{
				"title": title,
				"text":  msg.Text,
			},
		}, nil
	default:
		return nil, fmt.Errorf("unsupported dingtalk robot message type: %s", msgType)
	}
}

func effectiveRobotMessageType(msg gateway.ChannelResponse) string {
	switch msg.MessageType {
	case gateway.MessageTypeMarkdown:
		return "markdown"
	case gateway.MessageTypeText:
		if msg.Format == gateway.FormatMarkdown {
			return "markdown"
		}
		return "text"
	case gateway.MessageTypeImage:
		return "image"
	case gateway.MessageTypeAudio:
		return "audio"
	case gateway.MessageTypeVideo:
		return "video"
	case gateway.MessageTypeFile:
		return "file"
	}
	if msg.Format == gateway.FormatMarkdown {
		return "markdown"
	}
	return "text"
}
