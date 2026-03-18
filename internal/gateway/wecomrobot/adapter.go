package wecomrobot

import (
	"agentic-core/internal/gateway"
	"bytes"
	"context"
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const adapterName = "wecom_robot"

type Config struct {
	WebhookURL  string
	HTTPTimeout time.Duration
}

type Adapter struct {
	cfg        Config
	httpClient *http.Client
}

func NewAdapter(cfg Config, httpClient *http.Client) *Adapter {
	if cfg.HTTPTimeout <= 0 {
		cfg.HTTPTimeout = 10 * time.Second
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: cfg.HTTPTimeout}
	}
	return &Adapter{
		cfg:        cfg,
		httpClient: httpClient,
	}
}

func (a *Adapter) Name() string {
	return adapterName
}

func (a *Adapter) SendMessage(ctx context.Context, sessionID string, text string) error {
	return a.Send(ctx, gateway.ChannelResponse{
		SessionID:   sessionID,
		ChannelName: a.Name(),
		MessageType: gateway.MessageTypeText,
		Format:      gateway.FormatPlainText,
		Text:        text,
	})
}

func (a *Adapter) Send(ctx context.Context, msg gateway.ChannelResponse) error {
	webhookURL := a.resolveWebhookURL(msg)
	if webhookURL == "" {
		return fmt.Errorf("wecom robot webhook url is required")
	}

	payload, err := a.buildPayload(ctx, webhookURL, msg)
	if err != nil {
		return err
	}
	return a.postJSON(ctx, webhookURL, payload)
}

func (a *Adapter) buildPayload(ctx context.Context, webhookURL string, msg gateway.ChannelResponse) (map[string]any, error) {
	switch effectiveType(msg) {
	case "text":
		text := map[string]any{"content": msg.Text}
		if value, ok := msg.Metadata["mentioned_list"]; ok {
			text["mentioned_list"] = value
		}
		if value, ok := msg.Metadata["mentioned_mobile_list"]; ok {
			text["mentioned_mobile_list"] = value
		}
		return map[string]any{"msgtype": "text", "text": text}, nil
	case "markdown":
		return map[string]any{
			"msgtype":  "markdown",
			"markdown": map[string]any{"content": msg.Text},
		}, nil
	case "image":
		if len(msg.Media) == 0 {
			return nil, fmt.Errorf("wecom robot image message requires media")
		}
		base64Content, err := imageBase64(msg.Media[0])
		if err != nil {
			return nil, err
		}
		raw, err := base64.StdEncoding.DecodeString(base64Content)
		if err != nil {
			return nil, err
		}
		sum := md5.Sum(raw)
		return map[string]any{
			"msgtype": "image",
			"image": map[string]any{
				"base64": base64Content,
				"md5":    hex.EncodeToString(sum[:]),
			},
		}, nil
	case "file":
		if len(msg.Media) == 0 {
			return nil, fmt.Errorf("wecom robot file message requires media")
		}
		mediaID, err := a.uploadFile(ctx, webhookURL, msg.Media[0])
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"msgtype": "file",
			"file":    map[string]any{"media_id": mediaID},
		}, nil
	case "news":
		articles := make([]map[string]any, 0, len(msg.Articles))
		for _, article := range msg.Articles {
			articles = append(articles, map[string]any{
				"title":       article.Title,
				"description": article.Description,
				"url":         article.URL,
				"picurl":      article.PicURL,
			})
		}
		return map[string]any{
			"msgtype": "news",
			"news":    map[string]any{"articles": articles},
		}, nil
	default:
		return nil, fmt.Errorf("unsupported wecom robot message type")
	}
}

func effectiveType(msg gateway.ChannelResponse) string {
	switch msg.MessageType {
	case gateway.MessageTypeMarkdown:
		return "markdown"
	case gateway.MessageTypeImage:
		return "image"
	case gateway.MessageTypeFile:
		return "file"
	case gateway.MessageTypeNews:
		return "news"
	case gateway.MessageTypeText:
		return "text"
	}
	if msg.Format == gateway.FormatMarkdown {
		return "markdown"
	}
	if len(msg.Media) > 0 {
		switch msg.Media[0].Kind {
		case gateway.MediaKindImage:
			return "image"
		case gateway.MediaKindFile:
			return "file"
		}
	}
	if len(msg.Articles) > 0 {
		return "news"
	}
	return "text"
}

func imageBase64(media gateway.MediaItem) (string, error) {
	if strings.TrimSpace(media.DataBase64) != "" {
		return media.DataBase64, nil
	}
	if strings.TrimSpace(media.Path) != "" {
		raw, err := os.ReadFile(media.Path)
		if err != nil {
			return "", err
		}
		return base64.StdEncoding.EncodeToString(raw), nil
	}
	return "", fmt.Errorf("wecom robot image requires path or data_base64")
}

func (a *Adapter) uploadFile(ctx context.Context, webhookURL string, media gateway.MediaItem) (string, error) {
	uploadURL, err := webhookUploadURL(webhookURL)
	if err != nil {
		return "", err
	}

	content, fileName, err := mediaBytes(media)
	if err != nil {
		return "", err
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("media", fileName)
	if err != nil {
		return "", err
	}
	if _, err := part.Write(content); err != nil {
		return "", err
	}
	if err := writer.Close(); err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, uploadURL, &body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var parsed struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
		MediaID string `json:"media_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return "", err
	}
	if parsed.ErrCode != 0 {
		return "", fmt.Errorf("wecom robot upload_media failed: %s (%d)", parsed.ErrMsg, parsed.ErrCode)
	}
	if parsed.MediaID == "" {
		return "", fmt.Errorf("wecom robot upload_media returned empty media_id")
	}
	return parsed.MediaID, nil
}

func mediaBytes(media gateway.MediaItem) ([]byte, string, error) {
	if strings.TrimSpace(media.Path) != "" {
		raw, err := os.ReadFile(media.Path)
		if err != nil {
			return nil, "", err
		}
		name := media.FileName
		if strings.TrimSpace(name) == "" {
			name = filepath.Base(media.Path)
		}
		return raw, name, nil
	}
	if strings.TrimSpace(media.DataBase64) != "" {
		raw, err := base64.StdEncoding.DecodeString(media.DataBase64)
		if err != nil {
			return nil, "", err
		}
		name := media.FileName
		if strings.TrimSpace(name) == "" {
			name = "upload.bin"
		}
		return raw, name, nil
	}
	return nil, "", fmt.Errorf("wecom robot file requires path or data_base64")
}

func (a *Adapter) resolveWebhookURL(msg gateway.ChannelResponse) string {
	if len(msg.Metadata) != 0 {
		if value, ok := msg.Metadata["wecom_robot_webhook_url"]; ok {
			if raw := strings.TrimSpace(fmt.Sprint(value)); raw != "" {
				return raw
			}
		}
	}
	return strings.TrimSpace(a.cfg.WebhookURL)
}

func webhookUploadURL(webhookURL string) (string, error) {
	parsed, err := url.Parse(webhookURL)
	if err != nil {
		return "", err
	}
	key := parsed.Query().Get("key")
	if key == "" {
		return "", fmt.Errorf("wecom robot webhook url missing key")
	}
	parsed.Path = "/cgi-bin/webhook/upload_media"
	parsed.RawQuery = url.Values{"key": []string{key}}.Encode()
	return parsed.String(), nil
}

func (a *Adapter) postJSON(ctx context.Context, endpoint string, payload map[string]any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var parsed struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return err
	}
	if parsed.ErrCode != 0 {
		return fmt.Errorf("wecom robot send failed: %s (%d)", parsed.ErrMsg, parsed.ErrCode)
	}
	return nil
}

var _ gateway.RichAdapter = (*Adapter)(nil)
