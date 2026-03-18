package gateway

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	defaultGatewayHTTPPort           = ":8081"
	defaultGatewayRedisAddr          = "localhost:16379"
	defaultWeComCallbackPath         = "/callbacks/wecom"
	defaultFeishuAppEventCallback    = "/callbacks/feishu/events"
	defaultFeishuAppCardCallbackPath = "/callbacks/feishu/cards"
)

type Config struct {
	HTTPPort      string
	RedisAddr     string
	IngressSecret string
	WeCom         WeComConfig
	WeComRobot    WeComRobotConfig
	FeishuApp     FeishuAppConfig
	FeishuBot     FeishuBotConfig
}

type WeComConfig struct {
	CorpID             string
	AgentID            int64
	CorpSecret         string
	Token              string
	EncodingAESKey     string
	CallbackPath       string
	APIBaseURL         string
	MediaDir           string
	MediaRetentionDays int
}

type WeComRobotConfig struct {
	WebhookURL string
}

type FeishuAppConfig struct {
	Enabled           bool
	AppID             string
	AppSecret         string
	VerificationToken string
	EncryptKey        string
	EventCallbackPath string
	CardCallbackPath  string
	APIBaseURL        string
}

type FeishuBotConfig struct {
	Enabled    bool
	WebhookURL string
	Secret     string
}

func ParseConfig(args []string) (Config, error) {
	cfg := Config{
		HTTPPort:      getenvOrDefault("GATEWAY_HTTP_PORT", defaultGatewayHTTPPort),
		RedisAddr:     getenvOrDefault("GATEWAY_REDIS_ADDR", defaultGatewayRedisAddr),
		IngressSecret: strings.TrimSpace(os.Getenv("GATEWAY_INGRESS_SECRET")),
		WeCom: WeComConfig{
			CorpID:             strings.TrimSpace(os.Getenv("WECOM_CORP_ID")),
			AgentID:            getenvInt64("WECOM_AGENT_ID"),
			CorpSecret:         strings.TrimSpace(os.Getenv("WECOM_CORP_SECRET")),
			Token:              strings.TrimSpace(os.Getenv("WECOM_TOKEN")),
			EncodingAESKey:     strings.TrimSpace(os.Getenv("WECOM_ENCODING_AES_KEY")),
			CallbackPath:       getenvOrDefault("WECOM_CALLBACK_PATH", defaultWeComCallbackPath),
			APIBaseURL:         strings.TrimSpace(os.Getenv("WECOM_API_BASE_URL")),
			MediaDir:           getenvOrDefault("WECOM_MEDIA_DIR", defaultWeComMediaDir()),
			MediaRetentionDays: getenvInt("WECOM_MEDIA_RETENTION_DAYS"),
		},
		WeComRobot: WeComRobotConfig{
			WebhookURL: strings.TrimSpace(os.Getenv("WECOM_ROBOT_WEBHOOK_URL")),
		},
		FeishuApp: FeishuAppConfig{
			Enabled:           getenvBool("FEISHU_APP_ENABLED"),
			AppID:             strings.TrimSpace(os.Getenv("FEISHU_APP_ID")),
			AppSecret:         strings.TrimSpace(os.Getenv("FEISHU_APP_SECRET")),
			VerificationToken: strings.TrimSpace(os.Getenv("FEISHU_APP_VERIFICATION_TOKEN")),
			EncryptKey:        strings.TrimSpace(os.Getenv("FEISHU_APP_ENCRYPT_KEY")),
			EventCallbackPath: getenvOrDefault("FEISHU_APP_EVENT_CALLBACK_PATH", defaultFeishuAppEventCallback),
			CardCallbackPath:  getenvOrDefault("FEISHU_APP_CARD_CALLBACK_PATH", defaultFeishuAppCardCallbackPath),
			APIBaseURL:        strings.TrimSpace(os.Getenv("FEISHU_APP_API_BASE_URL")),
		},
		FeishuBot: FeishuBotConfig{
			Enabled:    getenvBool("FEISHU_BOT_ENABLED"),
			WebhookURL: strings.TrimSpace(os.Getenv("FEISHU_BOT_WEBHOOK_URL")),
			Secret:     strings.TrimSpace(os.Getenv("FEISHU_BOT_SECRET")),
		},
	}

	fs := flag.NewFlagSet("gateway", flag.ContinueOnError)
	fs.StringVar(&cfg.HTTPPort, "http-port", cfg.HTTPPort, "gateway http port")
	fs.StringVar(&cfg.RedisAddr, "redis-addr", cfg.RedisAddr, "redis address")
	fs.StringVar(&cfg.IngressSecret, "ingress-secret", cfg.IngressSecret, "shared secret for unified ingress signature verification")
	fs.StringVar(&cfg.WeCom.CorpID, "wecom-corp-id", cfg.WeCom.CorpID, "wecom corp id")
	fs.Int64Var(&cfg.WeCom.AgentID, "wecom-agent-id", cfg.WeCom.AgentID, "wecom agent id")
	fs.StringVar(&cfg.WeCom.CorpSecret, "wecom-corp-secret", cfg.WeCom.CorpSecret, "wecom corp secret")
	fs.StringVar(&cfg.WeCom.Token, "wecom-token", cfg.WeCom.Token, "wecom callback token")
	fs.StringVar(&cfg.WeCom.EncodingAESKey, "wecom-encoding-aes-key", cfg.WeCom.EncodingAESKey, "wecom callback encoding aes key")
	fs.StringVar(&cfg.WeCom.CallbackPath, "wecom-callback-path", cfg.WeCom.CallbackPath, "wecom callback path")
	fs.StringVar(&cfg.WeCom.APIBaseURL, "wecom-api-base-url", cfg.WeCom.APIBaseURL, "wecom api base url")
	fs.StringVar(&cfg.WeCom.MediaDir, "wecom-media-dir", cfg.WeCom.MediaDir, "wecom inbound media dir")
	fs.IntVar(&cfg.WeCom.MediaRetentionDays, "wecom-media-retention-days", cfg.WeCom.MediaRetentionDays, "wecom inbound media retention days (0 disables cleanup)")
	fs.StringVar(&cfg.WeComRobot.WebhookURL, "wecom-robot-webhook-url", cfg.WeComRobot.WebhookURL, "wecom robot webhook url")
	fs.BoolVar(&cfg.FeishuApp.Enabled, "feishu-app-enabled", cfg.FeishuApp.Enabled, "enable feishu app adapter")
	fs.StringVar(&cfg.FeishuApp.AppID, "feishu-app-id", cfg.FeishuApp.AppID, "feishu app id")
	fs.StringVar(&cfg.FeishuApp.AppSecret, "feishu-app-secret", cfg.FeishuApp.AppSecret, "feishu app secret")
	fs.StringVar(&cfg.FeishuApp.VerificationToken, "feishu-app-verification-token", cfg.FeishuApp.VerificationToken, "feishu app verification token")
	fs.StringVar(&cfg.FeishuApp.EncryptKey, "feishu-app-encrypt-key", cfg.FeishuApp.EncryptKey, "feishu app encrypt key")
	fs.StringVar(&cfg.FeishuApp.EventCallbackPath, "feishu-app-event-callback-path", cfg.FeishuApp.EventCallbackPath, "feishu app event callback path")
	fs.StringVar(&cfg.FeishuApp.CardCallbackPath, "feishu-app-card-callback-path", cfg.FeishuApp.CardCallbackPath, "feishu app card callback path")
	fs.StringVar(&cfg.FeishuApp.APIBaseURL, "feishu-app-api-base-url", cfg.FeishuApp.APIBaseURL, "feishu app api base url")
	fs.BoolVar(&cfg.FeishuBot.Enabled, "feishu-bot-enabled", cfg.FeishuBot.Enabled, "enable feishu bot adapter")
	fs.StringVar(&cfg.FeishuBot.WebhookURL, "feishu-bot-webhook-url", cfg.FeishuBot.WebhookURL, "feishu bot webhook url")
	fs.StringVar(&cfg.FeishuBot.Secret, "feishu-bot-secret", cfg.FeishuBot.Secret, "feishu bot secret")
	if err := fs.Parse(args); err != nil {
		return Config{}, err
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c Config) Validate() error {
	if strings.TrimSpace(c.HTTPPort) == "" {
		return fmt.Errorf("gateway http port is required")
	}
	if strings.TrimSpace(c.RedisAddr) == "" {
		return fmt.Errorf("gateway redis addr is required")
	}
	hasApp := c.WeCom.Enabled()
	hasRobot := c.WeComRobot.Enabled()
	hasFeishuApp := c.FeishuApp.IsEnabled()
	hasFeishuBot := c.FeishuBot.IsEnabled()
	if !hasApp && !hasRobot && !hasFeishuApp && !hasFeishuBot {
		return fmt.Errorf("at least one gateway channel must be configured")
	}
	if hasApp {
		if err := c.WeCom.Validate(); err != nil {
			return err
		}
	}
	if hasFeishuApp {
		if err := c.FeishuApp.Validate(); err != nil {
			return err
		}
	}
	if hasFeishuBot {
		if err := c.FeishuBot.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func (c WeComConfig) Enabled() bool {
	return strings.TrimSpace(c.CorpID) != "" ||
		c.AgentID > 0 ||
		strings.TrimSpace(c.CorpSecret) != "" ||
		strings.TrimSpace(c.Token) != "" ||
		strings.TrimSpace(c.EncodingAESKey) != ""
}

func (c WeComConfig) Validate() error {
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
	if strings.TrimSpace(c.CallbackPath) == "" {
		return fmt.Errorf("wecom callback path is required")
	}
	if c.MediaRetentionDays < 0 {
		return fmt.Errorf("wecom media retention days must be non-negative")
	}
	return nil
}

func (c WeComRobotConfig) Enabled() bool {
	return strings.TrimSpace(c.WebhookURL) != ""
}

func (c FeishuAppConfig) IsEnabled() bool {
	return c.Enabled ||
		strings.TrimSpace(c.AppID) != "" ||
		strings.TrimSpace(c.AppSecret) != "" ||
		strings.TrimSpace(c.VerificationToken) != "" ||
		strings.TrimSpace(c.EncryptKey) != ""
}

func (c FeishuAppConfig) Validate() error {
	if strings.TrimSpace(c.AppID) == "" {
		return fmt.Errorf("feishu app id is required")
	}
	if strings.TrimSpace(c.AppSecret) == "" {
		return fmt.Errorf("feishu app secret is required")
	}
	if strings.TrimSpace(c.VerificationToken) == "" {
		return fmt.Errorf("feishu app verification token is required")
	}
	if strings.TrimSpace(c.EncryptKey) == "" {
		return fmt.Errorf("feishu app encrypt key is required")
	}
	if strings.TrimSpace(c.EventCallbackPath) == "" {
		return fmt.Errorf("feishu app event callback path is required")
	}
	if strings.TrimSpace(c.CardCallbackPath) == "" {
		return fmt.Errorf("feishu app card callback path is required")
	}
	return nil
}

func (c FeishuBotConfig) IsEnabled() bool {
	return c.Enabled || strings.TrimSpace(c.WebhookURL) != ""
}

func (c FeishuBotConfig) Validate() error {
	if strings.TrimSpace(c.WebhookURL) == "" {
		return fmt.Errorf("feishu bot webhook url is required")
	}
	return nil
}

func getenvOrDefault(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func getenvInt64(key string) int64 {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return 0
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0
	}
	return value
}

func getenvInt(key string) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return 0
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0
	}
	return value
}

func getenvBool(key string) bool {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return false
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return false
	}
	return value
}

func defaultWeComMediaDir() string {
	return filepath.Join(os.TempDir(), "agentic-core", "wecom-media")
}
