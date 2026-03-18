package dingtalk

import (
	"agentic-core/internal/gateway"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type AppAdapter struct {
	cfg    AppConfig
	client appSender
}

func NewAppAdapter(cfg AppConfig, httpClient *http.Client) (*AppAdapter, error) {
	cfg = cfg.normalize()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	client, err := newAppClient(cfg, httpClient)
	if err != nil {
		return nil, err
	}
	return &AppAdapter{cfg: cfg, client: client}, nil
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

type appSender interface {
	Send(ctx context.Context, msg gateway.ChannelResponse) error
}

func (a *AppAdapter) EventHandler(router *gateway.SessionRouter) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a.handleCallback(router, w, r, mapEventCallback)
	})
}

func (a *AppAdapter) CardHandler(router *gateway.SessionRouter) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a.handleCallback(router, w, r, mapCardCallback)
	})
}

func (a *AppAdapter) handleCallback(router *gateway.SessionRouter, w http.ResponseWriter, r *http.Request, mapper func([]byte) (gateway.ChannelRequest, error)) {
	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}

	plain, challenge, unauthorized, encrypted, err := a.parseCallbackBody(r, body)
	if err != nil {
		if unauthorized {
			http.Error(w, "invalid signature", http.StatusUnauthorized)
			return
		}
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	if challenge != "" {
		if encrypted {
			if err := a.writeEncryptedResponse(w, r, []byte(challenge)); err != nil {
				http.Error(w, "invalid body", http.StatusBadRequest)
				return
			}
		} else {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{"challenge": challenge})
		}
		return
	}

	req, err := mapper(plain)
	if err != nil {
		http.Error(w, "invalid callback", http.StatusBadRequest)
		return
	}
	if err := router.HandleIncoming(r.Context(), req); err != nil {
		http.Error(w, "enqueue failed", http.StatusBadGateway)
		return
	}
	if encrypted {
		if err := a.writeEncryptedResponse(w, r, []byte("success")); err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (a *AppAdapter) parseCallbackBody(r *http.Request, body []byte) ([]byte, string, bool, bool, error) {
	challenge, err := extractChallenge(body)
	if err == nil && challenge != "" {
		return body, challenge, false, false, nil
	}

	var envelope encryptedCallbackEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, "", false, false, err
	}
	if envelope.Encrypt == "" {
		return body, "", false, false, nil
	}

	timestamp := r.URL.Query().Get("timestamp")
	nonce := r.URL.Query().Get("nonce")
	signature := firstNonEmpty(r.URL.Query().Get("signature"), r.URL.Query().Get("msg_signature"))
	if signature == "" || signature != callbackSignature(a.cfg.Token, timestamp, nonce, envelope.Encrypt) {
		return nil, "", true, true, fmt.Errorf("invalid signature")
	}

	plain, err := decryptCallbackBody(a.cfg.AESKey, a.cfg.ClientID, envelope.Encrypt)
	if err != nil {
		return nil, "", false, true, err
	}

	challenge, err = extractChallenge(plain)
	if err == nil && challenge != "" {
		return plain, challenge, false, true, nil
	}
	return plain, "", false, true, nil
}

func (a *AppAdapter) writeEncryptedResponse(w http.ResponseWriter, r *http.Request, body []byte) error {
	timestamp := firstNonEmpty(r.URL.Query().Get("timestamp"), "0")
	nonce := firstNonEmpty(r.URL.Query().Get("nonce"), "nonce")

	encrypted, err := encryptCallbackBody(a.cfg.AESKey, a.cfg.ClientID, body)
	if err != nil {
		return err
	}

	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(map[string]string{
		"msg_signature": callbackSignature(a.cfg.Token, timestamp, nonce, encrypted),
		"encrypt":       encrypted,
		"timeStamp":     timestamp,
		"nonce":         nonce,
	})
}
