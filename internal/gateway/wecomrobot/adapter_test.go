package wecomrobot

import (
	"context"
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"agentic-core/internal/gateway"
)

func TestAdapterSendMarkdownToWebhook(t *testing.T) {
	var requests []map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/cgi-bin/webhook/send" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("key") != "robot-key" {
			t.Fatalf("expected key robot-key, got %s", r.URL.Query().Get("key"))
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body failed: %v", err)
		}
		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("unmarshal payload failed: %v", err)
		}
		requests = append(requests, payload)
		_, _ = w.Write([]byte(`{"errcode":0,"errmsg":"ok"}`))
	}))
	defer server.Close()

	adapter := NewAdapter(Config{
		WebhookURL: server.URL + "/cgi-bin/webhook/send?key=robot-key",
	}, server.Client())

	err := adapter.Send(context.Background(), gateway.ChannelResponse{
		ChannelName: "wecom_robot",
		MessageType: gateway.MessageTypeMarkdown,
		Format:      gateway.FormatMarkdown,
		Text:        "**告警已恢复**",
	})
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	if len(requests) != 1 {
		t.Fatalf("expected 1 webhook request, got %d", len(requests))
	}
	if requests[0]["msgtype"] != "markdown" {
		t.Fatalf("expected markdown msgtype, got %v", requests[0]["msgtype"])
	}
	markdown, ok := requests[0]["markdown"].(map[string]any)
	if !ok {
		t.Fatalf("expected markdown payload, got %#v", requests[0]["markdown"])
	}
	if markdown["content"] != "**告警已恢复**" {
		t.Fatalf("expected markdown content preserved, got %v", markdown["content"])
	}
}

func TestAdapterUploadsFileAndSendsImagePayload(t *testing.T) {
	var uploadCalled bool
	var sendPayloads []map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cgi-bin/webhook/upload_media":
			uploadCalled = true
			if r.URL.Query().Get("key") != "robot-key" {
				t.Fatalf("expected key robot-key, got %s", r.URL.Query().Get("key"))
			}
			mediaType, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
			if err != nil {
				t.Fatalf("parse content type failed: %v", err)
			}
			if !strings.HasPrefix(mediaType, "multipart/") {
				t.Fatalf("expected multipart upload, got %s", mediaType)
			}
			reader := multipart.NewReader(r.Body, params["boundary"])
			part, err := reader.NextPart()
			if err != nil {
				t.Fatalf("read upload part failed: %v", err)
			}
			if part.FileName() != "report.txt" {
				t.Fatalf("expected upload file report.txt, got %s", part.FileName())
			}
			_, _ = io.Copy(io.Discard, part)
			_, _ = w.Write([]byte(`{"errcode":0,"errmsg":"ok","type":"file","media_id":"MEDIA_FILE_1","created_at":"1773811200"}`))
		case "/cgi-bin/webhook/send":
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read send body failed: %v", err)
			}
			var payload map[string]any
			if err := json.Unmarshal(body, &payload); err != nil {
				t.Fatalf("unmarshal send payload failed: %v", err)
			}
			sendPayloads = append(sendPayloads, payload)
			_, _ = w.Write([]byte(`{"errcode":0,"errmsg":"ok"}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	adapter := NewAdapter(Config{
		WebhookURL: server.URL + "/cgi-bin/webhook/send?key=robot-key",
	}, server.Client())

	err := adapter.Send(context.Background(), gateway.ChannelResponse{
		ChannelName: "wecom_robot",
		MessageType: gateway.MessageTypeImage,
		Media: []gateway.MediaItem{{
			Kind:       gateway.MediaKindImage,
			FileName:   "picture.png",
			DataBase64: base64.StdEncoding.EncodeToString([]byte("fake-image")),
		}},
	})
	if err != nil {
		t.Fatalf("image Send failed: %v", err)
	}

	err = adapter.Send(context.Background(), gateway.ChannelResponse{
		ChannelName: "wecom_robot",
		MessageType: gateway.MessageTypeFile,
		Media: []gateway.MediaItem{{
			Kind:       gateway.MediaKindFile,
			FileName:   "report.txt",
			DataBase64: base64.StdEncoding.EncodeToString([]byte("hello report")),
		}},
	})
	if err != nil {
		t.Fatalf("file Send failed: %v", err)
	}

	if !uploadCalled {
		t.Fatal("expected upload_media called for file payload")
	}
	if len(sendPayloads) != 2 {
		t.Fatalf("expected 2 send payloads, got %d", len(sendPayloads))
	}

	imagePayload := sendPayloads[0]
	if imagePayload["msgtype"] != "image" {
		t.Fatalf("expected image msgtype, got %v", imagePayload["msgtype"])
	}
	image, ok := imagePayload["image"].(map[string]any)
	if !ok {
		t.Fatalf("expected image payload, got %#v", imagePayload["image"])
	}
	rawImage := []byte("fake-image")
	sum := md5.Sum(rawImage)
	if image["base64"] != base64.StdEncoding.EncodeToString(rawImage) {
		t.Fatalf("expected base64 image content, got %v", image["base64"])
	}
	if image["md5"] != hex.EncodeToString(sum[:]) {
		t.Fatalf("expected image md5 %s, got %v", hex.EncodeToString(sum[:]), image["md5"])
	}

	filePayload := sendPayloads[1]
	if filePayload["msgtype"] != "file" {
		t.Fatalf("expected file msgtype, got %v", filePayload["msgtype"])
	}
	file, ok := filePayload["file"].(map[string]any)
	if !ok {
		t.Fatalf("expected file payload, got %#v", filePayload["file"])
	}
	if file["media_id"] != "MEDIA_FILE_1" {
		t.Fatalf("expected uploaded media id, got %v", file["media_id"])
	}
}
