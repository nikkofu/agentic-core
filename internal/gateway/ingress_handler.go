package gateway

import (
	"agentic-core/internal/skill"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"time"
)

type ingressResponse struct {
	Status string `json:"status"`
	TaskID string `json:"task_id,omitempty"`
}

type IngressHandlerConfig struct {
	Secret     string
	NonceStore skill.NonceStore
	Now        func() time.Time
}

func NewIngressHandler(router *SessionRouter, cfg ...IngressHandlerConfig) http.Handler {
	var options IngressHandlerConfig
	if len(cfg) > 0 {
		options = cfg[0]
	}
	if options.Now == nil {
		options.Now = time.Now
	}
	if options.Secret != "" && options.NonceStore == nil {
		options.NonceStore = skill.NewInMemNonceStore()
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		defer r.Body.Close()

		if options.Secret != "" {
			if err := skill.VerifyWebhookSignature(r.Header, body, options.Secret, options.NonceStore, options.Now()); err != nil {
				http.Error(w, "invalid signature", http.StatusUnauthorized)
				return
			}
		}

		var req ChannelRequest
		dec := json.NewDecoder(bytes.NewReader(body))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		if req.SessionID == "" || req.ChannelName == "" {
			http.Error(w, "session_id and channel_name are required", http.StatusBadRequest)
			return
		}

		taskID, err := router.DispatchIncoming(r.Context(), req)
		if err != nil {
			http.Error(w, "enqueue failed", http.StatusBadGateway)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(ingressResponse{
			Status: "accepted",
			TaskID: taskID,
		})
	})
}
