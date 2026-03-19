package feishu

import (
	"agentic-core/internal/gateway"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type httpDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

type BotClient struct {
	cfg        BotConfig
	httpClient httpDoer
	now        func() time.Time
}

func newBotClient(cfg BotConfig, httpClient *http.Client) *BotClient {
	cfg = cfg.normalize()
	if httpClient == nil {
		httpClient = &http.Client{Timeout: cfg.HTTPTimeout}
	}
	return &BotClient{
		cfg:        cfg,
		httpClient: httpClient,
		now:        time.Now,
	}
}

func (c *BotClient) Send(ctx context.Context, msg gateway.ChannelResponse) error {
	if err := c.cfg.Validate(); err != nil {
		return err
	}

	payload, err := buildBotPayload(msg, c.now(), c.cfg.Secret)
	if err != nil {
		return err
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.WebhookURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var parsed struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return err
	}
	if parsed.Code != 0 {
		return fmt.Errorf("feishu bot send failed: %s (%d)", parsed.Msg, parsed.Code)
	}
	return nil
}
