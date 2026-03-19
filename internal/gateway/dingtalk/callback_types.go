package dingtalk

type challengeRequest struct {
	Challenge string `json:"challenge"`
}

type textPayload struct {
	Content string `json:"content"`
}

type mediaPayload struct {
	MediaID  string `json:"mediaId"`
	FileName string `json:"fileName,omitempty"`
}

type eventCallback struct {
	Challenge      string       `json:"challenge,omitempty"`
	ConversationID string       `json:"conversationId"`
	MsgID          string       `json:"msgId"`
	ChatbotUserID  string       `json:"chatbotUserId"`
	SenderStaffID  string       `json:"senderStaffId"`
	SenderNick     string       `json:"senderNick"`
	MsgType        string       `json:"msgtype"`
	EventType      string       `json:"eventType"`
	Text           textPayload  `json:"text"`
	Image          mediaPayload `json:"image"`
	Audio          mediaPayload `json:"audio"`
	Video          mediaPayload `json:"video"`
	File           mediaPayload `json:"file"`
}

type cardCallback struct {
	Challenge      string         `json:"challenge,omitempty"`
	CardCallbackID string         `json:"cardCallbackId"`
	ConversationID string         `json:"conversationId"`
	MsgID          string         `json:"msgId"`
	ChatbotUserID  string         `json:"chatbotUserId"`
	SenderStaffID  string         `json:"senderStaffId"`
	Value          map[string]any `json:"value"`
	CardData       map[string]any `json:"cardData"`
}
