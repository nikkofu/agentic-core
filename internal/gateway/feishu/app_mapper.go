package feishu

import (
	"agentic-core/internal/gateway"
	"encoding/json"
	"fmt"
	larkcard "github.com/larksuite/oapi-sdk-go/v3/card"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	"strings"
)

func mapMessageEvent(event *larkim.P2MessageReceiveV1) (gateway.ChannelRequest, error) {
	if event == nil || event.Event == nil || event.Event.Message == nil {
		return gateway.ChannelRequest{}, fmt.Errorf("feishu message event is empty")
	}

	message := event.Event.Message
	req := gateway.ChannelRequest{
		SessionID:   firstNonEmpty(strValue(message.ChatId), senderOpenID(event.Event.Sender), senderUserID(event.Event.Sender)),
		ChannelName: appAdapterName,
		MessageID:   strValue(message.MessageId),
		SenderID:    firstNonEmpty(senderOpenID(event.Event.Sender), senderUserID(event.Event.Sender), senderUnionID(event.Event.Sender)),
		SenderName:  firstNonEmpty(senderOpenID(event.Event.Sender), senderUserID(event.Event.Sender), senderUnionID(event.Event.Sender)),
		Metadata: map[string]any{
			"event_id":        "",
			"tenant_key":      firstNonEmpty(event.TenantKey(), senderTenantKey(event.Event.Sender)),
			"chat_id":         strValue(message.ChatId),
			"thread_id":       strValue(message.ThreadId),
			"chat_type":       strValue(message.ChatType),
			"open_id":         senderOpenID(event.Event.Sender),
			"user_id":         senderUserID(event.Event.Sender),
			"union_id":        senderUnionID(event.Event.Sender),
			"receive_id_type": larkim.ReceiveIdTypeChatId,
		},
		RawContent: json.RawMessage(strValue(message.Content)),
	}
	if event.EventV2Base != nil && event.EventV2Base.Header != nil {
		req.Metadata["event_id"] = event.EventV2Base.Header.EventID
	}

	if req.SessionID == "" {
		req.SessionID = req.SenderID
	}

	switch strings.TrimSpace(strValue(message.MessageType)) {
	case "text":
		var content struct {
			Text string `json:"text"`
		}
		if err := json.Unmarshal([]byte(strValue(message.Content)), &content); err != nil {
			return gateway.ChannelRequest{}, err
		}
		req.MessageType = gateway.MessageTypeText
		req.Format = gateway.FormatPlainText
		req.Text = content.Text
	case "image":
		var content struct {
			ImageKey string `json:"image_key"`
		}
		if err := json.Unmarshal([]byte(strValue(message.Content)), &content); err != nil {
			return gateway.ChannelRequest{}, err
		}
		req.MessageType = gateway.MessageTypeImage
		req.Text = "[image]"
		req.Media = []gateway.MediaItem{{
			Kind:    gateway.MediaKindImage,
			MediaID: content.ImageKey,
		}}
	case "file":
		var content struct {
			FileKey  string `json:"file_key"`
			FileName string `json:"file_name"`
		}
		if err := json.Unmarshal([]byte(strValue(message.Content)), &content); err != nil {
			return gateway.ChannelRequest{}, err
		}
		req.MessageType = gateway.MessageTypeFile
		req.Text = "[file]"
		req.Media = []gateway.MediaItem{{
			Kind:     gateway.MediaKindFile,
			MediaID:  content.FileKey,
			FileName: content.FileName,
		}}
	default:
		req.MessageType = gateway.MessageTypeUnknown
		req.Text = strValue(message.Content)
	}

	return req, nil
}

func mapCardAction(cardAction *larkcard.CardAction) (gateway.ChannelRequest, error) {
	if cardAction == nil {
		return gateway.ChannelRequest{}, fmt.Errorf("feishu card action is empty")
	}

	cardBody, err := json.Marshal(cardAction)
	if err != nil {
		return gateway.ChannelRequest{}, err
	}

	req := gateway.ChannelRequest{
		SessionID:   firstNonEmpty(cardAction.OpenChatId, cardAction.OpenID, cardAction.UserID),
		ChannelName: appAdapterName,
		MessageID:   cardAction.OpenMessageID,
		SenderID:    firstNonEmpty(cardAction.OpenID, cardAction.UserID),
		SenderName:  firstNonEmpty(cardAction.OpenID, cardAction.UserID),
		Text:        "card_action",
		MessageType: gateway.MessageTypeEvent,
		Card: map[string]any{
			"open_message_id": cardAction.OpenMessageID,
			"open_chat_id":    cardAction.OpenChatId,
			"open_id":         cardAction.OpenID,
			"user_id":         cardAction.UserID,
			"tenant_key":      cardAction.TenantKey,
			"action":          cardAction.Action,
		},
		RawContent: cardBody,
		Metadata: map[string]any{
			"open_message_id": cardAction.OpenMessageID,
			"open_chat_id":    cardAction.OpenChatId,
			"open_id":         cardAction.OpenID,
			"user_id":         cardAction.UserID,
			"tenant_key":      cardAction.TenantKey,
			"action_tag":      "",
			"receive_id_type": larkim.ReceiveIdTypeChatId,
		},
	}
	if cardAction.Action != nil {
		req.Metadata["action_tag"] = cardAction.Action.Tag
		req.Metadata["action_value"] = cardAction.Action.Value
	}
	return req, nil
}

func senderOpenID(sender *larkim.EventSender) string {
	if sender == nil || sender.SenderId == nil {
		return ""
	}
	return strValue(sender.SenderId.OpenId)
}

func senderUserID(sender *larkim.EventSender) string {
	if sender == nil || sender.SenderId == nil {
		return ""
	}
	return strValue(sender.SenderId.UserId)
}

func senderUnionID(sender *larkim.EventSender) string {
	if sender == nil || sender.SenderId == nil {
		return ""
	}
	return strValue(sender.SenderId.UnionId)
}

func senderTenantKey(sender *larkim.EventSender) string {
	if sender == nil {
		return ""
	}
	return strValue(sender.TenantKey)
}

func strValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
