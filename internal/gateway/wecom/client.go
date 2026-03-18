package wecom

import (
	"agentic-core/internal/gateway"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type httpDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

type Client struct {
	cfg        Config
	httpClient httpDoer
	now        func() time.Time

	mu          sync.Mutex
	accessToken string
	expiresAt   time.Time
}

type DownloadedMedia struct {
	Path     string
	FileName string
	MIMEType string
	Size     int64
}

type apiResponse struct {
	ErrCode     int    `json:"errcode"`
	ErrMsg      string `json:"errmsg"`
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
	MediaID     string `json:"media_id"`
	Type        string `json:"type"`
	InvalidUser string `json:"invaliduser"`
}

func newClient(cfg Config, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: cfg.HTTPTimeout}
	}
	return &Client{
		cfg:        cfg,
		httpClient: httpClient,
		now:        time.Now,
	}
}

func (c *Client) Send(ctx context.Context, msg gateway.ChannelResponse) error {
	token, err := c.getAccessToken(ctx)
	if err != nil {
		return err
	}

	payload, err := c.buildPayload(ctx, msg)
	if err != nil {
		return err
	}

	resp, err := c.doJSON(ctx, http.MethodPost, "/cgi-bin/message/send", token, payload, nil)
	if err != nil {
		return err
	}
	if resp.InvalidUser != "" {
		return fmt.Errorf("wecom invalid user: %s", resp.InvalidUser)
	}
	return nil
}

func (c *Client) getAccessToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	if c.accessToken != "" && c.now().Before(c.expiresAt) {
		token := c.accessToken
		c.mu.Unlock()
		return token, nil
	}
	c.mu.Unlock()

	query := url.Values{}
	query.Set("corpid", c.cfg.CorpID)
	query.Set("corpsecret", c.cfg.CorpSecret)

	resp, err := c.doJSON(ctx, http.MethodGet, "/cgi-bin/gettoken", "", nil, query)
	if err != nil {
		return "", err
	}
	if resp.AccessToken == "" {
		return "", fmt.Errorf("wecom access token is empty")
	}

	expiry := c.now().Add(time.Duration(resp.ExpiresIn-60) * time.Second)
	c.mu.Lock()
	c.accessToken = resp.AccessToken
	c.expiresAt = expiry
	c.mu.Unlock()
	return resp.AccessToken, nil
}

func (c *Client) buildPayload(ctx context.Context, msg gateway.ChannelResponse) (map[string]any, error) {
	msgType := effectiveMessageType(msg)
	payload := map[string]any{
		"agentid": c.cfg.AgentID,
		"msgtype": msgType,
	}

	recipients := recipientFields(msg)
	for key, value := range recipients {
		payload[key] = value
	}
	if len(recipients) == 0 && msg.SessionID != "" {
		payload["touser"] = msg.SessionID
	}

	switch msgType {
	case "markdown":
		payload["markdown"] = map[string]any{"content": msg.Text}
	case "text":
		payload["text"] = map[string]any{"content": msg.Text}
	case "image", "voice", "video", "file":
		if len(msg.Media) == 0 {
			return nil, fmt.Errorf("wecom %s message requires media", msgType)
		}
		mediaID, err := c.resolveMediaID(ctx, msgType, msg.Media[0])
		if err != nil {
			return nil, err
		}
		switch msgType {
		case "image":
			payload["image"] = map[string]any{"media_id": mediaID}
		case "voice":
			payload["voice"] = map[string]any{"media_id": mediaID}
		case "video":
			payload["video"] = map[string]any{
				"media_id":    mediaID,
				"title":       msg.Media[0].Title,
				"description": msg.Media[0].Description,
			}
		case "file":
			payload["file"] = map[string]any{"media_id": mediaID}
		}
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
		payload["news"] = map[string]any{"articles": articles}
	default:
		return nil, fmt.Errorf("unsupported wecom message type: %s", msgType)
	}

	return payload, nil
}

func effectiveMessageType(msg gateway.ChannelResponse) string {
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
		return "voice"
	case gateway.MessageTypeVideo:
		return "video"
	case gateway.MessageTypeFile:
		return "file"
	case gateway.MessageTypeNews:
		return "news"
	}
	if msg.Format == gateway.FormatMarkdown {
		return "markdown"
	}
	if len(msg.Media) > 0 {
		switch msg.Media[0].Kind {
		case gateway.MediaKindImage:
			return "image"
		case gateway.MediaKindAudio:
			return "voice"
		case gateway.MediaKindVideo:
			return "video"
		case gateway.MediaKindFile:
			return "file"
		}
	}
	if len(msg.Articles) > 0 {
		return "news"
	}
	return "text"
}

func recipientFields(msg gateway.ChannelResponse) map[string]any {
	if len(msg.Metadata) == 0 {
		return nil
	}
	fields := map[string]any{}
	for _, key := range []string{"wecom_touser", "wecom_toparty", "wecom_totag", "wecom_chatid"} {
		if value, ok := msg.Metadata[key]; ok && value != nil && fmt.Sprint(value) != "" {
			switch key {
			case "wecom_touser":
				fields["touser"] = value
			case "wecom_toparty":
				fields["toparty"] = value
			case "wecom_totag":
				fields["totag"] = value
			case "wecom_chatid":
				fields["chatid"] = value
			}
		}
	}
	return fields
}

func (c *Client) resolveMediaID(ctx context.Context, msgType string, media gateway.MediaItem) (string, error) {
	if strings.TrimSpace(media.MediaID) != "" {
		return media.MediaID, nil
	}
	token, err := c.getAccessToken(ctx)
	if err != nil {
		return "", err
	}
	return c.uploadMedia(ctx, token, msgType, media)
}

func (c *Client) uploadMedia(ctx context.Context, token, msgType string, media gateway.MediaItem) (string, error) {
	content, fileName, err := materializeMedia(media)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
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

	query := url.Values{}
	query.Set("type", msgType)
	req, err := c.newRequest(ctx, http.MethodPost, "/cgi-bin/media/upload", token, &buf, query)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var parsed apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return "", err
	}
	if parsed.ErrCode != 0 {
		return "", fmt.Errorf("wecom media/upload failed: %s (%d)", parsed.ErrMsg, parsed.ErrCode)
	}
	if parsed.MediaID == "" {
		return "", fmt.Errorf("wecom media/upload returned empty media id")
	}
	return parsed.MediaID, nil
}

func materializeMedia(media gateway.MediaItem) ([]byte, string, error) {
	if strings.TrimSpace(media.Path) != "" {
		content, err := os.ReadFile(media.Path)
		if err != nil {
			return nil, "", err
		}
		name := media.FileName
		if strings.TrimSpace(name) == "" {
			name = filepath.Base(media.Path)
		}
		return content, name, nil
	}
	if strings.TrimSpace(media.DataBase64) != "" {
		content, err := base64.StdEncoding.DecodeString(media.DataBase64)
		if err != nil {
			return nil, "", err
		}
		name := media.FileName
		if strings.TrimSpace(name) == "" {
			name = "upload.bin"
		}
		return content, name, nil
	}
	return nil, "", fmt.Errorf("wecom media requires media_id, path or data_base64")
}

func (c *Client) DownloadMedia(ctx context.Context, mediaID string) (DownloadedMedia, error) {
	token, err := c.getAccessToken(ctx)
	if err != nil {
		return DownloadedMedia{}, err
	}

	query := url.Values{}
	query.Set("media_id", mediaID)
	req, err := c.newRequest(ctx, http.MethodGet, "/cgi-bin/media/get", token, nil, query)
	if err != nil {
		return DownloadedMedia{}, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return DownloadedMedia{}, err
	}
	defer resp.Body.Close()

	contentType := resp.Header.Get("Content-Type")
	if strings.Contains(strings.ToLower(contentType), "application/json") {
		var parsed apiResponse
		if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
			return DownloadedMedia{}, err
		}
		if parsed.ErrCode != 0 {
			return DownloadedMedia{}, fmt.Errorf("wecom media/get failed: %s (%d)", parsed.ErrMsg, parsed.ErrCode)
		}
		return DownloadedMedia{}, fmt.Errorf("wecom media/get returned json without file body")
	}

	if err := os.MkdirAll(c.cfg.MediaDir, 0o755); err != nil {
		return DownloadedMedia{}, err
	}

	fileName := filenameFromHeader(resp.Header.Get("Content-Disposition"))
	if fileName == "" {
		fileName = mediaID + extensionFromContentType(contentType)
	}
	targetPath := filepath.Join(c.cfg.MediaDir, fmt.Sprintf("%d_%s", c.now().UnixNano(), fileName))

	file, err := os.Create(targetPath)
	if err != nil {
		return DownloadedMedia{}, err
	}
	defer file.Close()

	size, err := io.Copy(file, resp.Body)
	if err != nil {
		return DownloadedMedia{}, err
	}

	return DownloadedMedia{
		Path:     targetPath,
		FileName: fileName,
		MIMEType: contentType,
		Size:     size,
	}, nil
}

func filenameFromHeader(contentDisposition string) string {
	if strings.TrimSpace(contentDisposition) == "" {
		return ""
	}
	_, params, err := mime.ParseMediaType(contentDisposition)
	if err != nil {
		return ""
	}
	return filepath.Base(params["filename"])
}

func extensionFromContentType(contentType string) string {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return ".bin"
	}
	exts, err := mime.ExtensionsByType(mediaType)
	if err != nil || len(exts) == 0 {
		return ".bin"
	}
	return exts[0]
}

func (c *Client) doJSON(ctx context.Context, method, path, token string, body any, query url.Values) (apiResponse, error) {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return apiResponse{}, err
		}
		reader = bytes.NewReader(raw)
	}
	req, err := c.newRequest(ctx, method, path, token, reader, query)
	if err != nil {
		return apiResponse{}, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return apiResponse{}, err
	}
	defer resp.Body.Close()

	var parsed apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return apiResponse{}, err
	}
	if parsed.ErrCode != 0 {
		return apiResponse{}, fmt.Errorf("wecom api failed: %s (%d)", parsed.ErrMsg, parsed.ErrCode)
	}
	return parsed, nil
}

func (c *Client) newRequest(ctx context.Context, method, path, token string, body io.Reader, query url.Values) (*http.Request, error) {
	base := strings.TrimRight(c.cfg.APIBaseURL, "/")
	values := url.Values{}
	for key, entries := range query {
		for _, entry := range entries {
			values.Add(key, entry)
		}
	}
	if token != "" {
		values.Set("access_token", token)
	}

	endpoint := base + path
	if encoded := values.Encode(); encoded != "" {
		endpoint += "?" + encoded
	}
	return http.NewRequestWithContext(ctx, method, endpoint, body)
}
