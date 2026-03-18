package feishu

import (
	"agentic-core/internal/gateway"
	"agentic-core/internal/logging"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

type appAPI interface {
	CreateMessage(ctx context.Context, receiveIDType, receiveID, msgType, content, uuid string) error
	UploadImage(ctx context.Context, fileName string, content []byte) (string, error)
	UploadFile(ctx context.Context, fileName string, content []byte) (string, error)
}

type sdkAppAPI struct {
	client *lark.Client
}

type AppClient struct {
	cfg  AppConfig
	api  appAPI
	uuid func() string
}

func newAppClient(cfg AppConfig, httpClient *http.Client) *AppClient {
	cfg = cfg.normalize()
	options := []lark.ClientOptionFunc{
		lark.WithReqTimeout(cfg.HTTPTimeout),
	}
	if cfg.APIBaseURL != "" {
		options = append(options, lark.WithOpenBaseUrl(cfg.APIBaseURL))
	}
	if httpClient != nil {
		options = append(options, lark.WithHttpClient(httpClient))
	}
	client := lark.NewClient(cfg.AppID, cfg.AppSecret, options...)
	return &AppClient{
		cfg:  cfg,
		api:  &sdkAppAPI{client: client},
		uuid: func() string { return fmt.Sprintf("feishu-%d", time.Now().UnixNano()) },
	}
}

func (c *AppClient) Send(ctx context.Context, msg gateway.ChannelResponse) error {
	receiveIDType := "chat_id"
	if value, ok := msg.Metadata["receive_id_type"]; ok && fmt.Sprint(value) != "" {
		receiveIDType = fmt.Sprint(value)
	}
	receiveID := msg.SessionID
	if receiveID == "" {
		receiveID = msg.ReceiverID
	}
	if receiveID == "" {
		return fmt.Errorf("feishu app receive id is required")
	}

	msgType, content, err := c.buildMessage(ctx, msg)
	if err != nil {
		return err
	}

	if err := c.api.CreateMessage(ctx, receiveIDType, receiveID, msgType, content, c.uuid()); err != nil {
		logging.Component("gateway.feishu_app").Error("feishu app send failed",
			"session_id", receiveID,
			"receive_id_type", receiveIDType,
			"message_type", msgType,
			"error", err.Error(),
		)
		return err
	}
	return nil
}

func (c *AppClient) buildMessage(ctx context.Context, msg gateway.ChannelResponse) (string, string, error) {
	switch {
	case msg.Card != nil:
		content, err := json.Marshal(msg.Card)
		if err != nil {
			return "", "", err
		}
		return "interactive", string(content), nil
	case msg.MessageType == gateway.MessageTypeMarkdown || msg.Format == gateway.FormatMarkdown:
		content, err := json.Marshal(map[string]any{
			"zh_cn": map[string]any{
				"title": "",
				"content": []any{
					[]any{
						map[string]string{"tag": "text", "text": msg.Text},
					},
				},
			},
		})
		if err != nil {
			return "", "", err
		}
		return "post", string(content), nil
	case msg.MessageType == gateway.MessageTypeImage || (len(msg.Media) > 0 && msg.Media[0].Kind == gateway.MediaKindImage):
		if len(msg.Media) == 0 {
			return "", "", fmt.Errorf("feishu image message requires media")
		}
		imageKey, err := c.resolveImageKey(ctx, msg.Media[0])
		if err != nil {
			return "", "", err
		}
		content, _ := json.Marshal(map[string]string{"image_key": imageKey})
		return "image", string(content), nil
	case msg.MessageType == gateway.MessageTypeFile || (len(msg.Media) > 0 && msg.Media[0].Kind == gateway.MediaKindFile):
		if len(msg.Media) == 0 {
			return "", "", fmt.Errorf("feishu file message requires media")
		}
		fileKey, err := c.resolveFileKey(ctx, msg.Media[0])
		if err != nil {
			return "", "", err
		}
		content, _ := json.Marshal(map[string]string{"file_key": fileKey})
		return "file", string(content), nil
	default:
		content, err := json.Marshal(map[string]string{"text": msg.Text})
		if err != nil {
			return "", "", err
		}
		return "text", string(content), nil
	}
}

func (c *AppClient) resolveImageKey(ctx context.Context, media gateway.MediaItem) (string, error) {
	if media.MediaID != "" {
		return media.MediaID, nil
	}
	content, fileName, err := mediaBytes(media)
	if err != nil {
		return "", err
	}
	return c.api.UploadImage(ctx, fileName, content)
}

func (c *AppClient) resolveFileKey(ctx context.Context, media gateway.MediaItem) (string, error) {
	if media.MediaID != "" {
		return media.MediaID, nil
	}
	content, fileName, err := mediaBytes(media)
	if err != nil {
		return "", err
	}
	return c.api.UploadFile(ctx, fileName, content)
}

func (s *sdkAppAPI) CreateMessage(ctx context.Context, receiveIDType, receiveID, msgType, content, uuid string) error {
	req := larkim.NewCreateMessageReqBuilder().
		ReceiveIdType(receiveIDType).
		Body(larkim.NewCreateMessageReqBodyBuilder().
			ReceiveId(receiveID).
			MsgType(msgType).
			Content(content).
			Uuid(uuid).
			Build()).
		Build()
	resp, err := s.client.Im.V1.Message.Create(ctx, req)
	if err != nil {
		return err
	}
	if !resp.Success() {
		return fmt.Errorf("feishu app create message failed: %s (%d)", resp.Msg, resp.Code)
	}
	return nil
}

func (s *sdkAppAPI) UploadImage(ctx context.Context, fileName string, content []byte) (string, error) {
	req := larkim.NewCreateImageReqBuilder().
		Body(larkim.NewCreateImageReqBodyBuilder().
			ImageType(larkim.ImageTypeMessage).
			Image(bytes.NewReader(content)).
			Build()).
		Build()
	resp, err := s.client.Im.V1.Image.Create(ctx, req)
	if err != nil {
		return "", err
	}
	if !resp.Success() || resp.Data == nil || resp.Data.ImageKey == nil || *resp.Data.ImageKey == "" {
		return "", fmt.Errorf("feishu app upload image failed: %s (%d)", resp.Msg, resp.Code)
	}
	return *resp.Data.ImageKey, nil
}

func (s *sdkAppAPI) UploadFile(ctx context.Context, fileName string, content []byte) (string, error) {
	req := larkim.NewCreateFileReqBuilder().
		Body(larkim.NewCreateFileReqBodyBuilder().
			FileType(larkim.FileTypeStream).
			FileName(fileName).
			File(bytes.NewReader(content)).
			Build()).
		Build()
	resp, err := s.client.Im.V1.File.Create(ctx, req)
	if err != nil {
		return "", err
	}
	if !resp.Success() || resp.Data == nil || resp.Data.FileKey == nil || *resp.Data.FileKey == "" {
		return "", fmt.Errorf("feishu app upload file failed: %s (%d)", resp.Msg, resp.Code)
	}
	return *resp.Data.FileKey, nil
}

func mediaBytes(media gateway.MediaItem) ([]byte, string, error) {
	if media.Path != "" {
		raw, err := os.ReadFile(media.Path)
		if err != nil {
			return nil, "", err
		}
		name := media.FileName
		if name == "" {
			name = filepath.Base(media.Path)
		}
		return raw, name, nil
	}
	if media.DataBase64 != "" {
		raw, err := base64.StdEncoding.DecodeString(media.DataBase64)
		if err != nil {
			return nil, "", err
		}
		name := media.FileName
		if name == "" {
			name = "upload.bin"
		}
		return raw, name, nil
	}
	return nil, "", fmt.Errorf("media requires path or data_base64")
}
