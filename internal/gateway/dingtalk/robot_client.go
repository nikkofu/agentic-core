package dingtalk

import (
	"agentic-core/internal/gateway"
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type robotHTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

type RobotClient struct {
	cfg        RobotConfig
	httpClient robotHTTPDoer
	now        func() time.Time
}

func newRobotClient(cfg RobotConfig, httpClient *http.Client) *RobotClient {
	cfg = cfg.normalize()
	if httpClient == nil {
		httpClient = &http.Client{Timeout: cfg.HTTPTimeout}
	}
	return &RobotClient{
		cfg:        cfg,
		httpClient: httpClient,
		now:        time.Now,
	}
}

func (c *RobotClient) Send(ctx context.Context, msg gateway.ChannelResponse) error {
	payload, err := buildRobotPayload(msg)
	if err != nil {
		return err
	}

	webhookURL := strings.TrimSpace(c.cfg.WebhookURL)
	if value, ok := msg.Metadata["dingtalk_robot_webhook_url"]; ok && fmt.Sprint(value) != "" {
		webhookURL = strings.TrimSpace(fmt.Sprint(value))
	}
	if webhookURL == "" {
		return fmt.Errorf("dingtalk robot webhook url is required")
	}

	secret := strings.TrimSpace(c.cfg.Secret)
	if value, ok := msg.Metadata["dingtalk_robot_secret"]; ok && fmt.Sprint(value) != "" {
		secret = strings.TrimSpace(fmt.Sprint(value))
	}

	if secret != "" {
		webhookURL, err = signWebhookURL(webhookURL, secret, c.now())
		if err != nil {
			return err
		}
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
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
		return fmt.Errorf("dingtalk robot send failed: %s (%d)", parsed.ErrMsg, parsed.ErrCode)
	}
	return nil
}

func signWebhookURL(rawURL, secret string, now time.Time) (string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}

	timestamp := fmt.Sprintf("%d", now.UnixMilli())
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(timestamp + "\n" + secret))
	sign := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	query := parsed.Query()
	query.Set("timestamp", timestamp)
	query.Set("sign", sign)
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}
