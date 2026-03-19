package dingtalk

import (
	"agentic-core/internal/gateway"
	"encoding/json"
	"fmt"
)

func mapEventCallback(body []byte) (gateway.ChannelRequest, error) {
	var callback eventCallback
	if err := json.Unmarshal(body, &callback); err != nil {
		return gateway.ChannelRequest{}, err
	}

	if err := requiredString("conversationId", firstNonEmpty(callback.ConversationID, callback.ChatbotUserID)); err != nil {
		return gateway.ChannelRequest{}, err
	}

	metadata := map[string]any{
		"conversationId": callback.ConversationID,
		"chatbotUserId":  callback.ChatbotUserID,
		"senderStaffId":  callback.SenderStaffID,
		"senderNick":     callback.SenderNick,
		"msgId":          callback.MsgID,
		"eventType":      callback.EventType,
		"msgtype":        callback.MsgType,
	}

	req := gateway.ChannelRequest{
		SessionID:   firstNonEmpty(callback.ConversationID, callback.ChatbotUserID),
		ChannelName: appAdapterName,
		MessageID:   callback.MsgID,
		SenderID:    callback.SenderStaffID,
		SenderName:  callback.SenderNick,
		Metadata:    metadata,
		RawContent:  append(json.RawMessage(nil), body...),
	}

	switch callback.MsgType {
	case "text":
		req.MessageType = gateway.MessageTypeText
		req.Format = gateway.FormatPlainText
		req.Text = callback.Text.Content
	case "image":
		req.MessageType = gateway.MessageTypeImage
		req.Text = "[image]"
		req.Media = []gateway.MediaItem{{
			Kind:    gateway.MediaKindImage,
			MediaID: callback.Image.MediaID,
		}}
	case "audio":
		req.MessageType = gateway.MessageTypeAudio
		req.Text = "[audio]"
		req.Media = []gateway.MediaItem{{
			Kind:    gateway.MediaKindAudio,
			MediaID: callback.Audio.MediaID,
		}}
	case "video":
		req.MessageType = gateway.MessageTypeVideo
		req.Text = "[video]"
		req.Media = []gateway.MediaItem{{
			Kind:    gateway.MediaKindVideo,
			MediaID: callback.Video.MediaID,
		}}
	case "file":
		req.MessageType = gateway.MessageTypeFile
		req.Text = "[file]"
		req.Media = []gateway.MediaItem{{
			Kind:     gateway.MediaKindFile,
			MediaID:  callback.File.MediaID,
			FileName: callback.File.FileName,
		}}
	default:
		if callback.EventType != "" {
			req.MessageType = gateway.MessageTypeEvent
			req.Text = callback.EventType
		} else {
			return gateway.ChannelRequest{}, fmt.Errorf("unsupported dingtalk event message type: %s", callback.MsgType)
		}
	}

	return req, nil
}

func mapCardCallback(body []byte) (gateway.ChannelRequest, error) {
	var callback cardCallback
	if err := json.Unmarshal(body, &callback); err != nil {
		return gateway.ChannelRequest{}, err
	}

	if err := requiredString("conversationId", firstNonEmpty(callback.ConversationID, callback.ChatbotUserID)); err != nil {
		return gateway.ChannelRequest{}, err
	}

	return gateway.ChannelRequest{
		SessionID:   firstNonEmpty(callback.ConversationID, callback.ChatbotUserID),
		ChannelName: appAdapterName,
		MessageID:   firstNonEmpty(callback.MsgID, callback.CardCallbackID),
		SenderID:    callback.SenderStaffID,
		Text:        "card_action",
		MessageType: gateway.MessageTypeEvent,
		Card: map[string]any{
			"value":    callback.Value,
			"cardData": callback.CardData,
		},
		RawContent: append(json.RawMessage(nil), body...),
		Metadata: map[string]any{
			"cardCallbackId": callback.CardCallbackID,
			"conversationId": callback.ConversationID,
			"chatbotUserId":  callback.ChatbotUserID,
			"senderStaffId":  callback.SenderStaffID,
			"msgId":          callback.MsgID,
		},
	}, nil
}
