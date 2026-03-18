package wecom

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	defaultCallbackPath = "/callbacks/wecom"
	defaultAPIBaseURL   = "https://qyapi.weixin.qq.com"
	defaultHTTPTimeout  = 10 * time.Second
)

type Config struct {
	CorpID         string
	AgentID        int64
	CorpSecret     string
	Token          string
	EncodingAESKey string
	CallbackPath   string
	APIBaseURL     string
	HTTPTimeout    time.Duration
	MediaDir       string
}

func (c Config) normalize() Config {
	cfg := c
	if strings.TrimSpace(cfg.CallbackPath) == "" {
		cfg.CallbackPath = defaultCallbackPath
	}
	if strings.TrimSpace(cfg.APIBaseURL) == "" {
		cfg.APIBaseURL = defaultAPIBaseURL
	}
	if cfg.HTTPTimeout <= 0 {
		cfg.HTTPTimeout = defaultHTTPTimeout
	}
	if strings.TrimSpace(cfg.MediaDir) == "" {
		cfg.MediaDir = filepath.Join(os.TempDir(), "agentic-core", "wecom-media")
	}
	return cfg
}

func (c Config) Validate() error {
	if strings.TrimSpace(c.CorpID) == "" {
		return fmt.Errorf("wecom corp id is required")
	}
	if c.AgentID <= 0 {
		return fmt.Errorf("wecom agent id must be positive")
	}
	if strings.TrimSpace(c.CorpSecret) == "" {
		return fmt.Errorf("wecom corp secret is required")
	}
	if strings.TrimSpace(c.Token) == "" {
		return fmt.Errorf("wecom token is required")
	}
	if strings.TrimSpace(c.EncodingAESKey) == "" {
		return fmt.Errorf("wecom encoding aes key is required")
	}
	return nil
}
