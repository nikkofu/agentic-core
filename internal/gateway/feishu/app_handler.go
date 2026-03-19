package feishu

import (
	"agentic-core/internal/gateway"
	"agentic-core/internal/logging"
	"context"
	"encoding/json"
	larkcard "github.com/larksuite/oapi-sdk-go/v3/card"
	larkevent "github.com/larksuite/oapi-sdk-go/v3/event"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	"io"
	"net/http"
)

const appAdapterName = "feishu_app"

type AppAdapter struct {
	cfg    AppConfig
	client *AppClient
}

func NewAppAdapter(cfg AppConfig, httpClient *http.Client) (*AppAdapter, error) {
	cfg = cfg.normalize()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &AppAdapter{
		cfg:    cfg,
		client: newAppClient(cfg, httpClient),
	}, nil
}

func (a *AppAdapter) Name() string {
	return appAdapterName
}

func (a *AppAdapter) SendMessage(ctx context.Context, sessionID string, text string) error {
	return a.Send(ctx, gateway.ChannelResponse{
		SessionID:   sessionID,
		ChannelName: a.Name(),
		MessageType: gateway.MessageTypeText,
		Format:      gateway.FormatPlainText,
		Text:        text,
	})
}

func (a *AppAdapter) Send(ctx context.Context, msg gateway.ChannelResponse) error {
	if msg.ChannelName == "" {
		msg.ChannelName = a.Name()
	}
	return a.client.Send(ctx, msg)
}

func (a *AppAdapter) EventHandler(router *gateway.SessionRouter) http.Handler {
	logger := logging.Component("gateway.feishu_app")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		defer r.Body.Close()

		plain, challenge, unauthorized, err := a.parseEventBody(r.Header, body)
		if err != nil {
			if unauthorized {
				http.Error(w, "invalid signature", http.StatusUnauthorized)
				return
			}
			http.Error(w, "invalid callback", http.StatusBadRequest)
			return
		}
		if challenge != "" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{"challenge": challenge})
			return
		}

		var event larkim.P2MessageReceiveV1
		if err := json.Unmarshal(plain, &event); err != nil {
			http.Error(w, "invalid callback", http.StatusBadRequest)
			return
		}

		req, err := mapMessageEvent(&event)
		if err != nil {
			http.Error(w, "invalid callback", http.StatusBadRequest)
			return
		}
		if _, err := router.DispatchIncoming(r.Context(), req); err != nil {
			http.Error(w, "enqueue failed", http.StatusBadGateway)
			return
		}

		logger.Info("feishu app callback accepted",
			"callback_kind", "message",
			"session_id", req.SessionID,
			"message_id", req.MessageID,
			"chat_id", req.Metadata["chat_id"],
			"event_id", req.Metadata["event_id"],
			"sender_id", req.SenderID,
		)
		w.WriteHeader(http.StatusOK)
	})
}

func (a *AppAdapter) CardHandler(router *gateway.SessionRouter) http.Handler {
	logger := logging.Component("gateway.feishu_app")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		defer r.Body.Close()

		plain, challenge, unauthorized, err := a.parseCardBody(r.Header, body)
		if err != nil {
			if unauthorized {
				http.Error(w, "invalid signature", http.StatusUnauthorized)
				return
			}
			http.Error(w, "invalid callback", http.StatusBadRequest)
			return
		}
		if challenge != "" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{"challenge": challenge})
			return
		}

		var cardAction larkcard.CardAction
		if err := json.Unmarshal(plain, &cardAction); err != nil {
			http.Error(w, "invalid callback", http.StatusBadRequest)
			return
		}

		req, err := mapCardAction(&cardAction)
		if err != nil {
			http.Error(w, "invalid callback", http.StatusBadRequest)
			return
		}
		if _, err := router.DispatchIncoming(r.Context(), req); err != nil {
			http.Error(w, "enqueue failed", http.StatusBadGateway)
			return
		}

		logger.Info("feishu app callback accepted",
			"callback_kind", "card_action",
			"session_id", req.SessionID,
			"message_id", req.MessageID,
			"chat_id", req.Metadata["open_chat_id"],
			"sender_id", req.SenderID,
		)
		w.WriteHeader(http.StatusOK)
	})
}

func (a *AppAdapter) parseEventBody(header http.Header, body []byte) ([]byte, string, bool, error) {
	plain, err := decryptEventBody(body, a.cfg.EncryptKey)
	if err != nil {
		return nil, "", false, err
	}

	var challenge struct {
		Type      string `json:"type"`
		Token     string `json:"token"`
		Challenge string `json:"challenge"`
	}
	if err := json.Unmarshal(plain, &challenge); err != nil {
		return nil, "", false, err
	}
	if challenge.Challenge != "" {
		if challenge.Token != a.cfg.VerificationToken {
			return nil, "", true, errUnauthorized
		}
		return plain, challenge.Challenge, false, nil
	}

	if a.cfg.EncryptKey != "" {
		if larkevent.Signature(header.Get(larkevent.EventRequestTimestamp), header.Get(larkevent.EventRequestNonce), a.cfg.EncryptKey, string(body)) != header.Get(larkevent.EventSignature) {
			return nil, "", true, errUnauthorized
		}
	}

	var envelope struct {
		Header *struct {
			Token string `json:"token"`
		} `json:"header"`
	}
	if err := json.Unmarshal(plain, &envelope); err != nil {
		return nil, "", false, err
	}
	if envelope.Header != nil && envelope.Header.Token != "" && envelope.Header.Token != a.cfg.VerificationToken {
		return nil, "", true, errUnauthorized
	}
	return plain, "", false, nil
}

func (a *AppAdapter) parseCardBody(header http.Header, body []byte) ([]byte, string, bool, error) {
	plain, err := decryptEventBody(body, a.cfg.EncryptKey)
	if err != nil {
		return nil, "", false, err
	}

	var challenge struct {
		Type      string `json:"type"`
		Token     string `json:"token"`
		Challenge string `json:"challenge"`
	}
	if err := json.Unmarshal(plain, &challenge); err != nil {
		return nil, "", false, err
	}
	if challenge.Challenge != "" {
		if challenge.Token != a.cfg.VerificationToken {
			return nil, "", true, errUnauthorized
		}
		return plain, challenge.Challenge, false, nil
	}

	if a.cfg.VerificationToken != "" {
		if larkcard.Signature(header.Get(larkevent.EventRequestTimestamp), header.Get(larkevent.EventRequestNonce), a.cfg.VerificationToken, string(body)) != header.Get(larkevent.EventSignature) {
			return nil, "", true, errUnauthorized
		}
	}
	return plain, "", false, nil
}
