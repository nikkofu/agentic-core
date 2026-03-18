package wecom

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"agentic-core/internal/gateway"
)

func TestAdapterSendUsesMarkdownPayload(t *testing.T) {
	var mu sync.Mutex
	tokenCalls := 0
	sendBodies := make([]map[string]any, 0, 2)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/cgi-bin/gettoken"):
			mu.Lock()
			tokenCalls++
			mu.Unlock()
			_, _ = w.Write([]byte(`{"errcode":0,"errmsg":"ok","access_token":"ACCESS_TOKEN","expires_in":7200}`))
		case strings.HasPrefix(r.URL.Path, "/cgi-bin/message/send"):
			defer r.Body.Close()
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read send body failed: %v", err)
			}
			var payload map[string]any
			if err := json.Unmarshal(body, &payload); err != nil {
				t.Fatalf("unmarshal send payload failed: %v", err)
			}
			mu.Lock()
			sendBodies = append(sendBodies, payload)
			mu.Unlock()
			_, _ = w.Write([]byte(`{"errcode":0,"errmsg":"ok","invaliduser":""}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	adapter, err := NewAdapter(Config{
		CorpID:         "ww-test-corp",
		AgentID:        1000002,
		CorpSecret:     "corp-secret",
		Token:          "gateway-token",
		EncodingAESKey: "abcdefghijklmnopqrstuvwxyz0123456789ABCDEFG",
		APIBaseURL:     server.URL,
	}, server.Client())
	if err != nil {
		t.Fatalf("NewAdapter failed: %v", err)
	}

	ctx := context.Background()
	for i := 0; i < 2; i++ {
		if err := adapter.Send(ctx, gateway.ChannelResponse{
			SessionID:   "zhangsan",
			ChannelName: "wecom",
			MessageType: gateway.MessageTypeMarkdown,
			Format:      gateway.FormatMarkdown,
			Text:        "**完成**",
		}); err != nil {
			t.Fatalf("Send failed: %v", err)
		}
	}

	if tokenCalls != 1 {
		t.Fatalf("expected token cached after first call, got %d token fetches", tokenCalls)
	}
	if len(sendBodies) != 2 {
		t.Fatalf("expected 2 send calls, got %d", len(sendBodies))
	}

	first := sendBodies[0]
	if first["msgtype"] != "markdown" {
		t.Fatalf("expected msgtype markdown, got %v", first["msgtype"])
	}
	if first["touser"] != "zhangsan" {
		t.Fatalf("expected touser zhangsan, got %v", first["touser"])
	}
	if int(first["agentid"].(float64)) != 1000002 {
		t.Fatalf("expected agentid 1000002, got %v", first["agentid"])
	}

	markdown, ok := first["markdown"].(map[string]any)
	if !ok {
		t.Fatalf("expected markdown payload, got %#v", first["markdown"])
	}
	if markdown["content"] != "**完成**" {
		t.Fatalf("expected markdown content preserved, got %v", markdown["content"])
	}
}

func TestAdapterSendUploadsMediaBeforeSending(t *testing.T) {
	var uploadedType string
	var uploadedFile string
	var sendPayload map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/cgi-bin/gettoken"):
			_, _ = w.Write([]byte(`{"errcode":0,"errmsg":"ok","access_token":"ACCESS_TOKEN","expires_in":7200}`))
		case strings.HasPrefix(r.URL.Path, "/cgi-bin/media/upload"):
			uploadedType = r.URL.Query().Get("type")
			contentType := r.Header.Get("Content-Type")
			_, params, err := mime.ParseMediaType(contentType)
			if err != nil {
				t.Fatalf("parse media content-type failed: %v", err)
			}
			reader := multipart.NewReader(r.Body, params["boundary"])
			part, err := reader.NextPart()
			if err != nil {
				t.Fatalf("read multipart part failed: %v", err)
			}
			uploadedFile = part.FileName()
			_, _ = io.Copy(io.Discard, part)
			_, _ = w.Write([]byte(`{"errcode":0,"errmsg":"ok","type":"image","media_id":"MEDIA_UP_1","created_at":"1773811200"}`))
		case strings.HasPrefix(r.URL.Path, "/cgi-bin/message/send"):
			defer r.Body.Close()
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read send body failed: %v", err)
			}
			if err := json.Unmarshal(body, &sendPayload); err != nil {
				t.Fatalf("unmarshal send payload failed: %v", err)
			}
			_, _ = w.Write([]byte(`{"errcode":0,"errmsg":"ok"}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	adapter, err := NewAdapter(Config{
		CorpID:         "ww-test-corp",
		AgentID:        1000002,
		CorpSecret:     "corp-secret",
		Token:          "gateway-token",
		EncodingAESKey: "abcdefghijklmnopqrstuvwxyz0123456789ABCDEFG",
		APIBaseURL:     server.URL,
	}, server.Client())
	if err != nil {
		t.Fatalf("NewAdapter failed: %v", err)
	}

	err = adapter.Send(context.Background(), gateway.ChannelResponse{
		SessionID:   "lisi",
		ChannelName: "wecom",
		MessageType: gateway.MessageTypeImage,
		Media: []gateway.MediaItem{
			{
				Kind:       gateway.MediaKindImage,
				FileName:   "a.jpg",
				DataBase64: base64.StdEncoding.EncodeToString([]byte("fake-image")),
			},
		},
	})
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	if uploadedType != "image" {
		t.Fatalf("expected upload type image, got %s", uploadedType)
	}
	if uploadedFile != "a.jpg" {
		t.Fatalf("expected uploaded file a.jpg, got %s", uploadedFile)
	}
	if sendPayload["msgtype"] != "image" {
		t.Fatalf("expected image msgtype, got %v", sendPayload["msgtype"])
	}
	imagePayload, ok := sendPayload["image"].(map[string]any)
	if !ok {
		t.Fatalf("expected image payload, got %#v", sendPayload["image"])
	}
	if imagePayload["media_id"] != "MEDIA_UP_1" {
		t.Fatalf("expected uploaded media id MEDIA_UP_1, got %v", imagePayload["media_id"])
	}
}
