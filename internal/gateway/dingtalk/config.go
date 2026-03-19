package dingtalk

import (
	"fmt"
	"strings"
	"time"
)

const (
	appAdapterName = "dingtalk_app"
	defaultTimeout = 10 * time.Second
)

type AppConfig struct {
	ClientID             string
	ClientSecret         string
	AgentID              int64
	EventCallbackPath    string
	CardCallbackPath     string
	APIBaseURL           string
	OAPIBaseURL          string
	Token                string
	AESKey               string
	MediaDir             string
	CardTemplateID       string
	CardCallbackRouteKey string
	HTTPTimeout          time.Duration
}

type RobotConfig struct {
	WebhookURL  string
	Secret      string
	HTTPTimeout time.Duration
}

func (c AppConfig) normalize() AppConfig {
	cfg := c
	if cfg.HTTPTimeout <= 0 {
		cfg.HTTPTimeout = defaultTimeout
	}
	return cfg
}

func (c AppConfig) Validate() error {
	if strings.TrimSpace(c.ClientID) == "" {
		return fmt.Errorf("dingtalk app client id is required")
	}
	if strings.TrimSpace(c.ClientSecret) == "" {
		return fmt.Errorf("dingtalk app client secret is required")
	}
	if c.AgentID <= 0 {
		return fmt.Errorf("dingtalk app agent id must be positive")
	}
	if strings.TrimSpace(c.Token) == "" {
		return fmt.Errorf("dingtalk app token is required")
	}
	if strings.TrimSpace(c.AESKey) == "" {
		return fmt.Errorf("dingtalk app aes key is required")
	}
	return nil
}

func (c RobotConfig) normalize() RobotConfig {
	cfg := c
	if cfg.HTTPTimeout <= 0 {
		cfg.HTTPTimeout = defaultTimeout
	}
	return cfg
}

func (c RobotConfig) Validate() error {
	if strings.TrimSpace(c.WebhookURL) == "" {
		return fmt.Errorf("dingtalk robot webhook url is required")
	}
	return nil
}
