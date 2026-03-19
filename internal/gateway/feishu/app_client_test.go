package feishu

import (
	"agentic-core/internal/gateway"
	"agentic-core/internal/logging"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type stubAppAPI struct {
	receiveIDType string
	receiveID     string
	msgType       string
	content       string
	uuid          string

	imageUploadCount int
	fileUploadCount  int
	imageKey         string
	fileKey          string

	sendErr        error
	uploadImageErr error
	uploadFileErr  error
}

func (s *stubAppAPI) CreateMessage(ctx context.Context, receiveIDType, receiveID, msgType, content, uuid string) error {
	s.receiveIDType = receiveIDType
	s.receiveID = receiveID
	s.msgType = msgType
	s.content = content
	s.uuid = uuid
	return s.sendErr
}

func (s *stubAppAPI) UploadImage(ctx context.Context, fileName string, content []byte) (string, error) {
	s.imageUploadCount++
	if s.uploadImageErr != nil {
		return "", s.uploadImageErr
	}
	if s.imageKey == "" {
		s.imageKey = "img_uploaded"
	}
	return s.imageKey, nil
}

func (s *stubAppAPI) UploadFile(ctx context.Context, fileName string, content []byte) (string, error) {
	s.fileUploadCount++
	if s.uploadFileErr != nil {
		return "", s.uploadFileErr
	}
	if s.fileKey == "" {
		s.fileKey = "file_uploaded"
	}
	return s.fileKey, nil
}

func TestAppClientSendsTextMessageToChatID(t *testing.T) {
	api := &stubAppAPI{}
	client := &AppClient{
		cfg:  AppConfig{AppID: "cli_app_id", AppSecret: "cli_secret"},
		api:  api,
		uuid: func() string { return "uuid-text" },
	}

	if err := client.Send(context.Background(), gateway.ChannelResponse{
		SessionID:   "oc_1",
		ChannelName: "feishu_app",
		MessageType: gateway.MessageTypeText,
		Text:        "hello app",
	}); err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	if api.receiveIDType != "chat_id" {
		t.Fatalf("expected chat_id receive type, got %s", api.receiveIDType)
	}
	if api.receiveID != "oc_1" {
		t.Fatalf("expected receive id oc_1, got %s", api.receiveID)
	}
	if api.msgType != "text" {
		t.Fatalf("expected text msg type, got %s", api.msgType)
	}

	var content map[string]any
	if err := json.Unmarshal([]byte(api.content), &content); err != nil {
		t.Fatalf("unmarshal content failed: %v", err)
	}
	if content["text"] != "hello app" {
		t.Fatalf("expected text content, got %#v", content["text"])
	}
}

func TestAppClientSendsCardMessage(t *testing.T) {
	api := &stubAppAPI{}
	client := &AppClient{
		cfg:  AppConfig{AppID: "cli_app_id", AppSecret: "cli_secret"},
		api:  api,
		uuid: func() string { return "uuid-card" },
	}

	if err := client.Send(context.Background(), gateway.ChannelResponse{
		SessionID:   "oc_card",
		ChannelName: "feishu_app",
		Card: map[string]any{
			"config": map[string]any{"wide_screen_mode": true},
			"elements": []any{
				map[string]any{"tag": "markdown", "content": "**done**"},
			},
		},
	}); err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	if api.msgType != "interactive" {
		t.Fatalf("expected interactive msg type, got %s", api.msgType)
	}

	var card map[string]any
	if err := json.Unmarshal([]byte(api.content), &card); err != nil {
		t.Fatalf("unmarshal card content failed: %v", err)
	}
	if _, ok := card["config"]; !ok {
		t.Fatalf("expected card config, got %#v", card)
	}
}

func TestAppClientUploadsImageBeforeSending(t *testing.T) {
	api := &stubAppAPI{imageKey: "img_v2_1"}
	client := &AppClient{
		cfg:  AppConfig{AppID: "cli_app_id", AppSecret: "cli_secret"},
		api:  api,
		uuid: func() string { return "uuid-image" },
	}

	raw := []byte("fake-image")
	if err := client.Send(context.Background(), gateway.ChannelResponse{
		SessionID:   "oc_image",
		ChannelName: "feishu_app",
		MessageType: gateway.MessageTypeImage,
		Media: []gateway.MediaItem{{
			Kind:       gateway.MediaKindImage,
			FileName:   "demo.png",
			DataBase64: base64.StdEncoding.EncodeToString(raw),
		}},
	}); err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	if api.imageUploadCount != 1 {
		t.Fatalf("expected one image upload, got %d", api.imageUploadCount)
	}
	if api.msgType != "image" {
		t.Fatalf("expected image msg type, got %s", api.msgType)
	}
	if !strings.Contains(api.content, "img_v2_1") {
		t.Fatalf("expected uploaded image key in content, got %s", api.content)
	}
}

func TestAppAdapterSendFallsBackToTextHelper(t *testing.T) {
	api := &stubAppAPI{}
	adapter := &AppAdapter{
		cfg: AppConfig{AppID: "cli_app_id", AppSecret: "cli_secret"},
		client: &AppClient{
			cfg:  AppConfig{AppID: "cli_app_id", AppSecret: "cli_secret"},
			api:  api,
			uuid: func() string { return "uuid-adapter" },
		},
	}

	if err := adapter.SendMessage(context.Background(), "oc_adapter", "hello from adapter"); err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}

	if api.receiveID != "oc_adapter" {
		t.Fatalf("expected receive id oc_adapter, got %s", api.receiveID)
	}
	if api.msgType != "text" {
		t.Fatalf("expected text msg type, got %s", api.msgType)
	}
}

func TestAppClientLogsNonSensitiveSendFailureMetadata(t *testing.T) {
	tmpDir := t.TempDir()
	_, err := logging.Init(logging.Config{
		Service:       "feishu-app-send-test",
		Dir:           tmpDir,
		Level:         logging.ParseLevel("info"),
		RetentionDays: 7,
		ConsoleEnable: false,
		FileEnable:    true,
		Now: func() time.Time {
			return time.Date(2026, 3, 18, 9, 0, 0, 0, time.UTC)
		},
	})
	if err != nil {
		t.Fatalf("Init logging failed: %v", err)
	}

	api := &stubAppAPI{sendErr: errors.New("upstream failed")}
	client := &AppClient{
		cfg: AppConfig{
			AppID:             "cli_app_id",
			AppSecret:         "cli-secret-value",
			VerificationToken: "verify-token-value",
		},
		api:  api,
		uuid: func() string { return "uuid-failure" },
	}

	err = client.Send(context.Background(), gateway.ChannelResponse{
		SessionID:   "oc_failure",
		ChannelName: "feishu_app",
		MessageType: gateway.MessageTypeText,
		Text:        "please fail",
	})
	if err == nil {
		t.Fatal("expected send failure")
	}

	logPath := filepath.Join(tmpDir, "2026-03-18", "feishu-app-send-test.jsonl")
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log file failed: %v", err)
	}

	raw := string(data)
	if !strings.Contains(raw, "gateway.feishu_app") || !strings.Contains(raw, "oc_failure") {
		t.Fatalf("expected session metadata in logs, got %s", raw)
	}
	if strings.Contains(raw, "cli-secret-value") || strings.Contains(raw, "verify-token-value") {
		t.Fatalf("expected secrets to stay out of logs, got %s", raw)
	}
}
