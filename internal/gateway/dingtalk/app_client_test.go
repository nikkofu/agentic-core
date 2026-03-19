package dingtalk

import (
	"agentic-core/internal/gateway"
	"context"
	"encoding/base64"
	"strings"
	"testing"
	"time"
)

type stubDingTalkAppAPI struct {
	token           appAccessToken
	tokenErr        error
	tokenCalls      int
	conversation    *appConversationRequest
	card            *appInteractiveCardRequest
	workNotice      *appWorkNoticeRequest
	uploads         []appUploadMediaRequest
	uploadedMedia   string
	conversationErr error
	cardErr         error
	workNoticeErr   error
	uploadErr       error
}

func (s *stubDingTalkAppAPI) GetAccessToken(ctx context.Context) (appAccessToken, error) {
	s.tokenCalls++
	if s.tokenErr != nil {
		return appAccessToken{}, s.tokenErr
	}
	if s.token.Token == "" {
		s.token.Token = "ding-access-token"
		s.token.ExpiresIn = 7200
	}
	return s.token, nil
}

func (s *stubDingTalkAppAPI) SendConversation(ctx context.Context, accessToken string, req appConversationRequest) error {
	copied := req
	s.conversation = &copied
	if s.conversationErr != nil {
		return s.conversationErr
	}
	return nil
}

func (s *stubDingTalkAppAPI) SendInteractiveCard(ctx context.Context, accessToken string, req appInteractiveCardRequest) error {
	copied := req
	s.card = &copied
	if s.cardErr != nil {
		return s.cardErr
	}
	return nil
}

func (s *stubDingTalkAppAPI) SendWorkNotice(ctx context.Context, accessToken string, req appWorkNoticeRequest) error {
	copied := req
	s.workNotice = &copied
	if s.workNoticeErr != nil {
		return s.workNoticeErr
	}
	return nil
}

func (s *stubDingTalkAppAPI) UploadMedia(ctx context.Context, accessToken string, req appUploadMediaRequest) (string, error) {
	s.uploads = append(s.uploads, req)
	if s.uploadErr != nil {
		return "", s.uploadErr
	}
	if s.uploadedMedia == "" {
		s.uploadedMedia = "media-uploaded"
	}
	return s.uploadedMedia, nil
}

func TestAppClientSendsTextMessage(t *testing.T) {
	api := &stubDingTalkAppAPI{}
	client := &AppClient{
		cfg: AppConfig{
			ClientID:     "ding-app-id",
			ClientSecret: "ding-secret",
			AgentID:      900001,
		},
		api: api,
		id:  func() string { return "track-text" },
		now: func() time.Time { return time.Unix(1700000000, 0) },
	}

	err := client.Send(context.Background(), gateway.ChannelResponse{
		SessionID:   "cid-text",
		ChannelName: appAdapterName,
		MessageType: gateway.MessageTypeText,
		Text:        "hello ding",
		Metadata: map[string]any{
			"chatbotUserId": "dingbot-user-1",
		},
	})
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	if api.tokenCalls != 1 {
		t.Fatalf("expected one token request, got %d", api.tokenCalls)
	}
	if api.conversation == nil {
		t.Fatal("expected conversation send request")
	}
	if api.conversation.Sender != "dingbot-user-1" {
		t.Fatalf("expected sender dingbot-user-1, got %s", api.conversation.Sender)
	}
	if api.conversation.CID != "cid-text" {
		t.Fatalf("expected cid cid-text, got %s", api.conversation.CID)
	}
	if api.conversation.Msg["msgtype"] != "text" {
		t.Fatalf("expected text msgtype, got %#v", api.conversation.Msg["msgtype"])
	}
	textBody, ok := api.conversation.Msg["text"].(map[string]any)
	if !ok {
		t.Fatalf("expected text payload, got %#v", api.conversation.Msg["text"])
	}
	if textBody["content"] != "hello ding" {
		t.Fatalf("expected content hello ding, got %#v", textBody["content"])
	}
}

func TestAppClientSendsMarkdownMessage(t *testing.T) {
	api := &stubDingTalkAppAPI{}
	client := &AppClient{
		cfg: AppConfig{
			ClientID:     "ding-app-id",
			ClientSecret: "ding-secret",
			AgentID:      900001,
		},
		api: api,
		id:  func() string { return "track-markdown" },
		now: func() time.Time { return time.Unix(1700000000, 0) },
	}

	err := client.Send(context.Background(), gateway.ChannelResponse{
		SessionID:   "cid-md",
		ChannelName: appAdapterName,
		MessageType: gateway.MessageTypeMarkdown,
		Format:      gateway.FormatMarkdown,
		Text:        "## deploy ok",
		Metadata: map[string]any{
			"title":         "Deploy Status",
			"chatbotUserId": "dingbot-user-2",
		},
	})
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	if api.conversation == nil {
		t.Fatal("expected conversation send request")
	}
	if api.conversation.Sender != "dingbot-user-2" {
		t.Fatalf("expected sender dingbot-user-2, got %s", api.conversation.Sender)
	}
	if api.conversation.Msg["msgtype"] != "markdown" {
		t.Fatalf("expected markdown msgtype, got %#v", api.conversation.Msg["msgtype"])
	}
	markdownBody, ok := api.conversation.Msg["markdown"].(map[string]any)
	if !ok {
		t.Fatalf("expected markdown payload, got %#v", api.conversation.Msg["markdown"])
	}
	if markdownBody["title"] != "Deploy Status" {
		t.Fatalf("expected markdown title, got %#v", markdownBody["title"])
	}
	if markdownBody["text"] != "## deploy ok" {
		t.Fatalf("expected markdown text, got %#v", markdownBody["text"])
	}
}

func TestAppClientSendsCardMessage(t *testing.T) {
	api := &stubDingTalkAppAPI{}
	client := &AppClient{
		cfg: AppConfig{
			ClientID:             "ding-app-id",
			ClientSecret:         "ding-secret",
			AgentID:              900001,
			CardCallbackRouteKey: "card-route-default",
		},
		api: api,
		id:  func() string { return "track-card" },
		now: func() time.Time { return time.Unix(1700000000, 0) },
	}

	err := client.Send(context.Background(), gateway.ChannelResponse{
		SessionID:   "cid-card",
		ChannelName: appAdapterName,
		Card: map[string]any{
			"cardTemplateId":   "tpl-123",
			"conversationType": 2,
			"callbackRouteKey": "card-route-override",
			"supportForward":   true,
			"cardParamMap": map[string]any{
				"title":  "Deploy OK",
				"status": "done",
			},
		},
		Metadata: map[string]any{
			"chatbotUserId": "dingbot-user-card",
		},
	})
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	if api.card == nil {
		t.Fatal("expected interactive card request")
	}
	if api.card.OpenConversationID != "cid-card" {
		t.Fatalf("expected open conversation id cid-card, got %s", api.card.OpenConversationID)
	}
	if api.card.ChatBotID != "dingbot-user-card" {
		t.Fatalf("expected chatbot id dingbot-user-card, got %s", api.card.ChatBotID)
	}
	if api.card.CardTemplateID != "tpl-123" {
		t.Fatalf("expected card template tpl-123, got %s", api.card.CardTemplateID)
	}
	if api.card.OutTrackID != "track-card" {
		t.Fatalf("expected out track id track-card, got %s", api.card.OutTrackID)
	}
	if api.card.CallbackRouteKey != "card-route-override" {
		t.Fatalf("expected callback route override, got %s", api.card.CallbackRouteKey)
	}
	if !api.card.SupportForward {
		t.Fatal("expected support forward to be true")
	}
	if api.card.CardParamMap["title"] != "Deploy OK" {
		t.Fatalf("expected title param, got %#v", api.card.CardParamMap["title"])
	}
}

func TestAppClientUploadsMediaBeforeSending(t *testing.T) {
	api := &stubDingTalkAppAPI{uploadedMedia: "media-image-1"}
	client := &AppClient{
		cfg: AppConfig{
			ClientID:     "ding-app-id",
			ClientSecret: "ding-secret",
		},
		api: api,
		id:  func() string { return "track-image" },
		now: func() time.Time { return time.Unix(1700000000, 0) },
	}

	raw := []byte("fake-image")
	err := client.Send(context.Background(), gateway.ChannelResponse{
		ChannelName: appAdapterName,
		MessageType: gateway.MessageTypeImage,
		SessionID:   "cid-image",
		Media: []gateway.MediaItem{{
			Kind:       gateway.MediaKindImage,
			FileName:   "demo.png",
			DataBase64: base64.StdEncoding.EncodeToString(raw),
		}},
		Metadata: map[string]any{
			"chatbotUserId": "dingbot-user-image",
		},
	})
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	if len(api.uploads) != 1 {
		t.Fatalf("expected one media upload, got %d", len(api.uploads))
	}
	if api.uploads[0].MediaType != "image" {
		t.Fatalf("expected image upload type, got %s", api.uploads[0].MediaType)
	}
	if api.uploads[0].FileName != "demo.png" {
		t.Fatalf("expected file name demo.png, got %s", api.uploads[0].FileName)
	}

	if api.conversation == nil {
		t.Fatal("expected conversation request")
	}
	if api.conversation.Sender != "dingbot-user-image" {
		t.Fatalf("expected sender dingbot-user-image, got %s", api.conversation.Sender)
	}
	if api.conversation.CID != "cid-image" {
		t.Fatalf("expected cid-image target, got %s", api.conversation.CID)
	}
	if api.conversation.Msg["msgtype"] != "image" {
		t.Fatalf("expected conversation msgtype image, got %#v", api.conversation.Msg["msgtype"])
	}
	imageBody, ok := api.conversation.Msg["image"].(map[string]any)
	if !ok {
		t.Fatalf("expected image payload, got %#v", api.conversation.Msg["image"])
	}
	if imageBody["media_id"] != "media-image-1" {
		t.Fatalf("expected uploaded media id, got %#v", imageBody["media_id"])
	}
}

func TestAppAdapterSendFallsBackToTextHelper(t *testing.T) {
	stub := &stubAppSender{}
	adapter := &AppAdapter{
		cfg: AppConfig{
			ClientID:     "ding-app-id",
			ClientSecret: "ding-secret",
			AgentID:      900001,
		},
		client: stub,
	}

	if err := adapter.SendMessage(context.Background(), "cid-adapter", "hello adapter"); err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}

	if len(stub.messages) != 1 {
		t.Fatalf("expected one message, got %d", len(stub.messages))
	}
	if stub.messages[0].SessionID != "cid-adapter" {
		t.Fatalf("expected session id cid-adapter, got %s", stub.messages[0].SessionID)
	}
	if stub.messages[0].Text != "hello adapter" {
		t.Fatalf("expected hello adapter, got %s", stub.messages[0].Text)
	}
}

func TestAppClientRejectsMissingReceiveTarget(t *testing.T) {
	client := &AppClient{
		cfg: AppConfig{
			ClientID:     "ding-app-id",
			ClientSecret: "ding-secret",
			AgentID:      900001,
		},
		api: &stubDingTalkAppAPI{},
		id:  func() string { return "track-missing-target" },
		now: func() time.Time { return time.Unix(1700000000, 0) },
	}

	err := client.Send(context.Background(), gateway.ChannelResponse{
		ChannelName: appAdapterName,
		MessageType: gateway.MessageTypeText,
		Text:        "hello",
	})
	if err == nil {
		t.Fatal("expected missing target error")
	}
	if !strings.Contains(err.Error(), "dingtalk app receive target is required") {
		t.Fatalf("expected missing target error, got %v", err)
	}
}

type stubAppSender struct {
	messages []gateway.ChannelResponse
}

func (s *stubAppSender) Send(ctx context.Context, msg gateway.ChannelResponse) error {
	s.messages = append(s.messages, msg)
	return nil
}
