package feishu

import (
	"fmt"
	"strings"
	"time"
)

const (
	defaultHTTPTimeout = 10 * time.Second
)

type BotConfig struct {
	WebhookURL  string
	Secret      string
	HTTPTimeout time.Duration
}

type AppConfig struct {
	AppID             string
	AppSecret         string
	VerificationToken string
	EncryptKey        string
	EventCallbackPath string
	CardCallbackPath  string
	APIBaseURL        string
	HTTPTimeout       time.Duration
}

func (c BotConfig) normalize() BotConfig {
	cfg := c
	if cfg.HTTPTimeout <= 0 {
		cfg.HTTPTimeout = defaultHTTPTimeout
	}
	return cfg
}

func (c BotConfig) Validate() error {
	if strings.TrimSpace(c.WebhookURL) == "" {
		return fmt.Errorf("feishu bot webhook url is required")
	}
	return nil
}

func (c AppConfig) normalize() AppConfig {
	cfg := c
	if cfg.HTTPTimeout <= 0 {
		cfg.HTTPTimeout = defaultHTTPTimeout
	}
	return cfg
}

func (c AppConfig) Validate() error {
	if strings.TrimSpace(c.AppID) == "" {
		return fmt.Errorf("feishu app id is required")
	}
	if strings.TrimSpace(c.AppSecret) == "" {
		return fmt.Errorf("feishu app secret is required")
	}
	if strings.TrimSpace(c.VerificationToken) == "" {
		return fmt.Errorf("feishu app verification token is required")
	}
	return nil
}
