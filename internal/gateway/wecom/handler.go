package wecom

import (
	"agentic-core/internal/gateway"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type Adapter struct {
	cfg    Config
	codec  *Codec
	client *Client
}

func NewAdapter(cfg Config, httpClient *http.Client) (*Adapter, error) {
	cfg = cfg.normalize()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	codec, err := NewCodec(cfg)
	if err != nil {
		return nil, err
	}
	return &Adapter{
		cfg:    cfg,
		codec:  codec,
		client: newClient(cfg, httpClient),
	}, nil
}

func (a *Adapter) Name() string {
	return "wecom"
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
	return a.client.Send(ctx, msg)
}

func (a *Adapter) CallbackHandler(router *gateway.SessionRouter) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			a.handleVerifyURL(w, r)
		case http.MethodPost:
			a.handleCallback(router, w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
}

func (a *Adapter) handleVerifyURL(w http.ResponseWriter, r *http.Request) {
	plain, err := a.codec.VerifyURL(
		r.URL.Query().Get("msg_signature"),
		r.URL.Query().Get("timestamp"),
		r.URL.Query().Get("nonce"),
		r.URL.Query().Get("echostr"),
	)
	if err != nil {
		http.Error(w, "invalid signature", http.StatusUnauthorized)
		return
	}
	_, _ = w.Write([]byte(plain))
}

func (a *Adapter) handleCallback(router *gateway.SessionRouter, w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}

	var envelope callbackEnvelope
	if err := xml.Unmarshal(body, &envelope); err != nil {
		http.Error(w, "invalid xml", http.StatusBadRequest)
		return
	}

	plain, err := a.codec.Decrypt(
		r.URL.Query().Get("msg_signature"),
		r.URL.Query().Get("timestamp"),
		r.URL.Query().Get("nonce"),
		envelope.Encrypt,
	)
	if err != nil {
		http.Error(w, "invalid signature", http.StatusUnauthorized)
		return
	}

	var msg callbackMessage
	if err := xml.Unmarshal(plain, &msg); err != nil {
		http.Error(w, "invalid callback xml", http.StatusBadRequest)
		return
	}

	standard := toStandardMessage(msg)
	a.enrichInboundMedia(r.Context(), &standard)

	if err := router.HandleIncoming(r.Context(), standard); err != nil {
		http.Error(w, "enqueue failed", http.StatusBadGateway)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func toStandardMessage(msg callbackMessage) gateway.ChannelRequest {
	metadata := map[string]any{
		"wecom_msg_type":    msg.MsgType,
		"wecom_agent_id":    msg.AgentID,
		"wecom_to_user":     msg.ToUserName,
		"wecom_from_user":   msg.FromUserName,
		"wecom_create_time": msg.CreateTime,
	}

	req := gateway.ChannelRequest{
		SessionID:   msg.FromUserName,
		ChannelName: "wecom",
		MessageID:   msg.MsgID,
		SenderID:    msg.FromUserName,
		SenderName:  msg.FromUserName,
		ReceiverID:  msg.ToUserName,
		Metadata:    metadata,
	}

	switch strings.ToLower(strings.TrimSpace(msg.MsgType)) {
	case "text":
		req.MessageType = gateway.MessageTypeText
		req.Format = gateway.FormatPlainText
		req.Text = msg.Content
	case "image":
		req.MessageType = gateway.MessageTypeImage
		req.Text = "[image]"
		req.Media = []gateway.MediaItem{{
			Kind:    gateway.MediaKindImage,
			MediaID: msg.MediaID,
			URL:     msg.PicURL,
		}}
	case "voice":
		req.MessageType = gateway.MessageTypeAudio
		req.Text = "[audio]"
		req.Media = []gateway.MediaItem{{
			Kind:    gateway.MediaKindAudio,
			MediaID: msg.MediaID,
			Metadata: map[string]any{
				"format": msg.Format,
			},
		}}
	case "video":
		req.MessageType = gateway.MessageTypeVideo
		req.Text = "[video]"
		req.Media = []gateway.MediaItem{{
			Kind:             gateway.MediaKindVideo,
			MediaID:          msg.MediaID,
			ThumbnailMediaID: msg.ThumbMediaID,
		}}
	case "file":
		req.MessageType = gateway.MessageTypeFile
		req.Text = "[file]"
		req.Media = []gateway.MediaItem{{
			Kind:    gateway.MediaKindFile,
			MediaID: msg.MediaID,
		}}
	case "link":
		req.MessageType = gateway.MessageTypeLink
		req.Text = strings.TrimSpace(strings.Join([]string{msg.Title, msg.Description, msg.URL}, "\n"))
		req.Metadata["title"] = msg.Title
		req.Metadata["description"] = msg.Description
		req.Metadata["url"] = msg.URL
	case "location":
		req.MessageType = gateway.MessageTypeLocation
		req.Text = msg.Label
		req.Metadata["location_x"] = msg.LocationX
		req.Metadata["location_y"] = msg.LocationY
		req.Metadata["scale"] = msg.Scale
	case "event":
		req.MessageType = gateway.MessageTypeEvent
		req.Text = msg.Event
		req.Metadata["event"] = msg.Event
		req.Metadata["change_type"] = msg.ChangeType
	default:
		req.MessageType = gateway.MessageTypeUnknown
		req.Text = msg.Content
	}

	if req.MessageID == "" {
		req.MessageID = fmt.Sprintf("%s-%d", req.MessageType, msg.CreateTime)
	}
	return req
}

func (a *Adapter) enrichInboundMedia(ctx context.Context, msg *gateway.ChannelRequest) {
	if len(msg.Media) == 0 {
		return
	}
	for idx := range msg.Media {
		media := &msg.Media[idx]
		if strings.TrimSpace(media.MediaID) == "" || strings.TrimSpace(media.Path) != "" {
			continue
		}
		downloaded, err := a.client.DownloadMedia(ctx, media.MediaID)
		if err != nil {
			if media.Metadata == nil {
				media.Metadata = map[string]any{}
			}
			media.Metadata["download_error"] = err.Error()
			continue
		}
		media.Path = downloaded.Path
		media.FileName = downloaded.FileName
		media.MIMEType = downloaded.MIMEType
		media.Size = downloaded.Size
	}
}
