package wecom

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"agentic-core/internal/bus"
	"agentic-core/internal/gateway"
)

func TestHandlerParsesTextAndMediaCallbacksIntoStandardMessage(t *testing.T) {
	tests := []struct {
		name           string
		innerXML       string
		wantType       string
		wantText       string
		wantMediaCount int
		wantFirstMedia string
		wantMediaKind  string
		wantMetadata   map[string]string
	}{
		{
			name: "text",
			innerXML: `<xml>
<ToUserName><![CDATA[ww-test-corp]]></ToUserName>
<FromUserName><![CDATA[zhangsan]]></FromUserName>
<CreateTime>1773811200</CreateTime>
<MsgType><![CDATA[text]]></MsgType>
<Content><![CDATA[你好，网关]]></Content>
<MsgId>1001</MsgId>
<AgentID>1000002</AgentID>
</xml>`,
			wantType: "text",
			wantText: "你好，网关",
		},
		{
			name: "image",
			innerXML: `<xml>
<ToUserName><![CDATA[ww-test-corp]]></ToUserName>
<FromUserName><![CDATA[lisi]]></FromUserName>
<CreateTime>1773811200</CreateTime>
<MsgType><![CDATA[image]]></MsgType>
<PicUrl><![CDATA[https://example.com/a.jpg]]></PicUrl>
<MediaId><![CDATA[MEDIA_IMAGE_1]]></MediaId>
<MsgId>1002</MsgId>
<AgentID>1000002</AgentID>
</xml>`,
			wantType:       "image",
			wantMediaCount: 1,
			wantFirstMedia: "MEDIA_IMAGE_1",
			wantMediaKind:  "image",
		},
		{
			name: "voice",
			innerXML: `<xml>
<ToUserName><![CDATA[ww-test-corp]]></ToUserName>
<FromUserName><![CDATA[wangwu]]></FromUserName>
<CreateTime>1773811200</CreateTime>
<MsgType><![CDATA[voice]]></MsgType>
<MediaId><![CDATA[MEDIA_AUDIO_1]]></MediaId>
<Format><![CDATA[amr]]></Format>
<MsgId>1003</MsgId>
<AgentID>1000002</AgentID>
</xml>`,
			wantType:       "audio",
			wantMediaCount: 1,
			wantFirstMedia: "MEDIA_AUDIO_1",
			wantMediaKind:  "audio",
		},
		{
			name: "video",
			innerXML: `<xml>
<ToUserName><![CDATA[ww-test-corp]]></ToUserName>
<FromUserName><![CDATA[zhaoliu]]></FromUserName>
<CreateTime>1773811200</CreateTime>
<MsgType><![CDATA[video]]></MsgType>
<MediaId><![CDATA[MEDIA_VIDEO_1]]></MediaId>
<ThumbMediaId><![CDATA[MEDIA_VIDEO_THUMB_1]]></ThumbMediaId>
<MsgId>1004</MsgId>
<AgentID>1000002</AgentID>
</xml>`,
			wantType:       "video",
			wantMediaCount: 1,
			wantFirstMedia: "MEDIA_VIDEO_1",
			wantMediaKind:  "video",
		},
		{
			name: "file",
			innerXML: `<xml>
<ToUserName><![CDATA[ww-test-corp]]></ToUserName>
<FromUserName><![CDATA[tianqi]]></FromUserName>
<CreateTime>1773811200</CreateTime>
<MsgType><![CDATA[file]]></MsgType>
<MediaId><![CDATA[MEDIA_FILE_1]]></MediaId>
<MsgId>1005</MsgId>
<AgentID>1000002</AgentID>
</xml>`,
			wantType:       "file",
			wantMediaCount: 1,
			wantFirstMedia: "MEDIA_FILE_1",
			wantMediaKind:  "file",
		},
		{
			name: "link",
			innerXML: `<xml>
<ToUserName><![CDATA[ww-test-corp]]></ToUserName>
<FromUserName><![CDATA[qianqi]]></FromUserName>
<CreateTime>1773811200</CreateTime>
<MsgType><![CDATA[link]]></MsgType>
<Title><![CDATA[报警详情]]></Title>
<Description><![CDATA[主机 CPU 超过阈值]]></Description>
<Url><![CDATA[https://example.com/ticket/42]]></Url>
<MsgId>1006</MsgId>
<AgentID>1000002</AgentID>
</xml>`,
			wantType: "link",
			wantText: "报警详情\n主机 CPU 超过阈值\nhttps://example.com/ticket/42",
			wantMetadata: map[string]string{
				"title":       "报警详情",
				"description": "主机 CPU 超过阈值",
				"url":         "https://example.com/ticket/42",
			},
		},
		{
			name: "location",
			innerXML: `<xml>
<ToUserName><![CDATA[ww-test-corp]]></ToUserName>
<FromUserName><![CDATA[sunba]]></FromUserName>
<CreateTime>1773811200</CreateTime>
<MsgType><![CDATA[location]]></MsgType>
<Location_X>31.2304</Location_X>
<Location_Y>121.4737</Location_Y>
<Scale>15</Scale>
<Label><![CDATA[上海市黄浦区]]></Label>
<MsgId>1007</MsgId>
<AgentID>1000002</AgentID>
</xml>`,
			wantType: "location",
			wantText: "上海市黄浦区",
			wantMetadata: map[string]string{
				"location_x": "31.2304",
				"location_y": "121.4737",
				"scale":      "15",
			},
		},
		{
			name: "event",
			innerXML: `<xml>
<ToUserName><![CDATA[ww-test-corp]]></ToUserName>
<FromUserName><![CDATA[jiujiu]]></FromUserName>
<CreateTime>1773811200</CreateTime>
<MsgType><![CDATA[event]]></MsgType>
<Event><![CDATA[change_contact]]></Event>
<ChangeType><![CDATA[create_user]]></ChangeType>
<AgentID>1000002</AgentID>
</xml>`,
			wantType: "event",
			wantText: "change_contact",
			wantMetadata: map[string]string{
				"event":       "change_contact",
				"change_type": "create_user",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := bus.NewFakeTransport()
			router := gateway.NewSessionRouter(transport)

			cfg := Config{
				CorpID:         "ww-test-corp",
				AgentID:        1000002,
				CorpSecret:     "corp-secret",
				Token:          "gateway-token",
				EncodingAESKey: "abcdefghijklmnopqrstuvwxyz0123456789ABCDEFG",
			}
			adapter, err := NewAdapter(cfg, nil)
			if err != nil {
				t.Fatalf("NewAdapter failed: %v", err)
			}

			codec, err := NewCodec(cfg)
			if err != nil {
				t.Fatalf("NewCodec failed: %v", err)
			}
			encrypted, err := codec.Encrypt([]byte(tt.innerXML))
			if err != nil {
				t.Fatalf("Encrypt failed: %v", err)
			}

			timestamp := "1773811200"
			nonce := "nonce-handler"
			signature := codec.Signature(timestamp, nonce, encrypted)
			body := fmt.Sprintf(`<xml><ToUserName><![CDATA[ww-test-corp]]></ToUserName><AgentID><![CDATA[1000002]]></AgentID><Encrypt><![CDATA[%s]]></Encrypt></xml>`, encrypted)

			req := httptest.NewRequest(http.MethodPost, "/callbacks/wecom?msg_signature="+signature+"&timestamp="+timestamp+"&nonce="+nonce, strings.NewReader(body))
			rec := httptest.NewRecorder()
			adapter.CallbackHandler(router).ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
			}

			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			msgChan, err := transport.Dequeue(ctx, "tasks")
			if err != nil {
				t.Fatalf("Dequeue failed: %v", err)
			}

			var enqueued bus.Message
			select {
			case enqueued = <-msgChan:
			case <-ctx.Done():
				t.Fatal("timed out waiting for task")
			}

			var payload struct {
				Task        string              `json:"task"`
				Channel     string              `json:"channel"`
				SessionID   string              `json:"session_id"`
				MessageType string              `json:"message_type"`
				Media       []gateway.MediaItem `json:"media"`
				Metadata    map[string]any      `json:"metadata"`
			}
			if err := json.Unmarshal(enqueued.Payload, &payload); err != nil {
				t.Fatalf("unmarshal payload failed: %v", err)
			}

			if payload.Channel != "wecom" {
				t.Fatalf("expected wecom channel, got %s", payload.Channel)
			}
			if payload.MessageType != tt.wantType {
				t.Fatalf("expected message type %s, got %s", tt.wantType, payload.MessageType)
			}
			if tt.wantText != "" && payload.Task != tt.wantText {
				t.Fatalf("expected text %s, got %s", tt.wantText, payload.Task)
			}
			if len(payload.Media) != tt.wantMediaCount {
				t.Fatalf("expected %d media items, got %d", tt.wantMediaCount, len(payload.Media))
			}
			if tt.wantMediaCount > 0 {
				if payload.Media[0].MediaID != tt.wantFirstMedia {
					t.Fatalf("expected media id %s, got %s", tt.wantFirstMedia, payload.Media[0].MediaID)
				}
				if string(payload.Media[0].Kind) != tt.wantMediaKind {
					t.Fatalf("expected media kind %s, got %s", tt.wantMediaKind, payload.Media[0].Kind)
				}
			}
			for key, want := range tt.wantMetadata {
				if got := fmt.Sprint(payload.Metadata[key]); got != want {
					t.Fatalf("expected metadata %s=%s, got %s", key, want, got)
				}
			}
		})
	}
}

func TestHandlerDownloadsInboundMediaToConfiguredDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	var tokenCalls int
	var downloadCalls int

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/cgi-bin/gettoken"):
			tokenCalls++
			_, _ = w.Write([]byte(`{"errcode":0,"errmsg":"ok","access_token":"ACCESS_TOKEN","expires_in":7200}`))
		case strings.HasPrefix(r.URL.Path, "/cgi-bin/media/get"):
			downloadCalls++
			w.Header().Set("Content-Type", "image/jpeg")
			w.Header().Set("Content-Disposition", `attachment; filename="photo.jpg"`)
			_, _ = w.Write([]byte("fake-image-binary"))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	transport := bus.NewFakeTransport()
	router := gateway.NewSessionRouter(transport)

	cfg := Config{
		CorpID:         "ww-test-corp",
		AgentID:        1000002,
		CorpSecret:     "corp-secret",
		Token:          "gateway-token",
		EncodingAESKey: "abcdefghijklmnopqrstuvwxyz0123456789ABCDEFG",
		APIBaseURL:     server.URL,
		MediaDir:       tmpDir,
	}
	adapter, err := NewAdapter(cfg, server.Client())
	if err != nil {
		t.Fatalf("NewAdapter failed: %v", err)
	}

	codec, err := NewCodec(cfg)
	if err != nil {
		t.Fatalf("NewCodec failed: %v", err)
	}
	innerXML := `<xml>
<ToUserName><![CDATA[ww-test-corp]]></ToUserName>
<FromUserName><![CDATA[lisi]]></FromUserName>
<CreateTime>1773811200</CreateTime>
<MsgType><![CDATA[image]]></MsgType>
<PicUrl><![CDATA[https://example.com/a.jpg]]></PicUrl>
<MediaId><![CDATA[MEDIA_IMAGE_2]]></MediaId>
<MsgId>1006</MsgId>
<AgentID>1000002</AgentID>
</xml>`
	encrypted, err := codec.Encrypt([]byte(innerXML))
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}
	timestamp := "1773811200"
	nonce := "nonce-download"
	signature := codec.Signature(timestamp, nonce, encrypted)
	body := fmt.Sprintf(`<xml><ToUserName><![CDATA[ww-test-corp]]></ToUserName><AgentID><![CDATA[1000002]]></AgentID><Encrypt><![CDATA[%s]]></Encrypt></xml>`, encrypted)

	req := httptest.NewRequest(http.MethodPost, "/callbacks/wecom?msg_signature="+signature+"&timestamp="+timestamp+"&nonce="+nonce, strings.NewReader(body))
	rec := httptest.NewRecorder()
	adapter.CallbackHandler(router).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if tokenCalls != 1 {
		t.Fatalf("expected one token call, got %d", tokenCalls)
	}
	if downloadCalls != 1 {
		t.Fatalf("expected one media download call, got %d", downloadCalls)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	msgChan, err := transport.Dequeue(ctx, "tasks")
	if err != nil {
		t.Fatalf("Dequeue failed: %v", err)
	}

	var enqueued bus.Message
	select {
	case enqueued = <-msgChan:
	case <-ctx.Done():
		t.Fatal("timed out waiting for task")
	}

	var payload struct {
		Media []gateway.MediaItem `json:"media"`
	}
	if err := json.Unmarshal(enqueued.Payload, &payload); err != nil {
		t.Fatalf("unmarshal payload failed: %v", err)
	}
	if len(payload.Media) != 1 {
		t.Fatalf("expected one media item, got %d", len(payload.Media))
	}
	if payload.Media[0].Path == "" {
		t.Fatal("expected downloaded media path recorded")
	}
	if payload.Media[0].FileName != "photo.jpg" {
		t.Fatalf("expected filename photo.jpg, got %s", payload.Media[0].FileName)
	}
	if payload.Media[0].MIMEType != "image/jpeg" {
		t.Fatalf("expected mime image/jpeg, got %s", payload.Media[0].MIMEType)
	}

	data, err := os.ReadFile(payload.Media[0].Path)
	if err != nil {
		t.Fatalf("read downloaded file failed: %v", err)
	}
	if string(data) != "fake-image-binary" {
		t.Fatalf("unexpected downloaded file content: %s", string(data))
	}
	if !strings.HasPrefix(payload.Media[0].Path, tmpDir) {
		t.Fatalf("expected media stored under %s, got %s", tmpDir, payload.Media[0].Path)
	}
}
