package gateway

import "testing"

func TestParseConfigUsesEnvAndFlags(t *testing.T) {
	t.Setenv("GATEWAY_HTTP_PORT", ":8089")
	t.Setenv("GATEWAY_REDIS_ADDR", "redis.internal:6379")
	t.Setenv("WECOM_CORP_ID", "ww-env")
	t.Setenv("WECOM_AGENT_ID", "1000002")
	t.Setenv("WECOM_CORP_SECRET", "env-secret")
	t.Setenv("WECOM_TOKEN", "env-token")
	t.Setenv("WECOM_ENCODING_AES_KEY", "abcdefghijklmnopqrstuvwxyz0123456789ABCDEFG")
	t.Setenv("WECOM_CALLBACK_PATH", "/env/callback")
	t.Setenv("WECOM_MEDIA_DIR", "/tmp/wecom-media")
	t.Setenv("GATEWAY_INGRESS_SECRET", "env-ingress-secret")

	cfg, err := ParseConfig([]string{
		"-http-port=:9091",
		"-wecom-agent-id=1000010",
	})
	if err != nil {
		t.Fatalf("ParseConfig failed: %v", err)
	}

	if cfg.HTTPPort != ":9091" {
		t.Fatalf("expected flag override http port, got %s", cfg.HTTPPort)
	}
	if cfg.RedisAddr != "redis.internal:6379" {
		t.Fatalf("expected redis env, got %s", cfg.RedisAddr)
	}
	if cfg.WeCom.CorpID != "ww-env" {
		t.Fatalf("expected corp id from env, got %s", cfg.WeCom.CorpID)
	}
	if cfg.WeCom.AgentID != 1000010 {
		t.Fatalf("expected flag override agent id, got %d", cfg.WeCom.AgentID)
	}
	if cfg.WeCom.CallbackPath != "/env/callback" {
		t.Fatalf("expected callback path from env, got %s", cfg.WeCom.CallbackPath)
	}
	if cfg.WeCom.MediaDir != "/tmp/wecom-media" {
		t.Fatalf("expected media dir from env, got %s", cfg.WeCom.MediaDir)
	}
	if cfg.IngressSecret != "env-ingress-secret" {
		t.Fatalf("expected ingress secret from env, got %s", cfg.IngressSecret)
	}
}

func TestParseConfigAcceptsRobotOnlyMode(t *testing.T) {
	t.Setenv("GATEWAY_HTTP_PORT", ":8089")
	t.Setenv("GATEWAY_REDIS_ADDR", "redis.internal:6379")
	t.Setenv("WECOM_ROBOT_WEBHOOK_URL", "https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=robot-key")

	cfg, err := ParseConfig(nil)
	if err != nil {
		t.Fatalf("ParseConfig failed: %v", err)
	}

	if cfg.WeCom.CorpID != "" {
		t.Fatalf("expected app wecom config empty in robot-only mode, got %s", cfg.WeCom.CorpID)
	}
	if cfg.WeComRobot.WebhookURL == "" {
		t.Fatal("expected robot webhook url present")
	}
}

func TestParseConfigUsesMediaRetentionDays(t *testing.T) {
	t.Setenv("GATEWAY_HTTP_PORT", ":8089")
	t.Setenv("GATEWAY_REDIS_ADDR", "redis.internal:6379")
	t.Setenv("WECOM_CORP_ID", "ww-env")
	t.Setenv("WECOM_AGENT_ID", "1000002")
	t.Setenv("WECOM_CORP_SECRET", "env-secret")
	t.Setenv("WECOM_TOKEN", "env-token")
	t.Setenv("WECOM_ENCODING_AES_KEY", "abcdefghijklmnopqrstuvwxyz0123456789ABCDEFG")
	t.Setenv("WECOM_MEDIA_RETENTION_DAYS", "7")

	cfg, err := ParseConfig([]string{
		"-wecom-media-retention-days=3",
	})
	if err != nil {
		t.Fatalf("ParseConfig failed: %v", err)
	}

	if cfg.WeCom.MediaRetentionDays != 3 {
		t.Fatalf("expected retention days flag override, got %d", cfg.WeCom.MediaRetentionDays)
	}
}

func TestParseConfigAllowsFeishuAppOnlyMode(t *testing.T) {
	t.Setenv("GATEWAY_HTTP_PORT", ":8089")
	t.Setenv("GATEWAY_REDIS_ADDR", "redis.internal:6379")
	t.Setenv("FEISHU_APP_ENABLED", "true")
	t.Setenv("FEISHU_APP_ID", "cli_app_id")
	t.Setenv("FEISHU_APP_SECRET", "cli_app_secret")
	t.Setenv("FEISHU_APP_VERIFICATION_TOKEN", "verify-token")
	t.Setenv("FEISHU_APP_ENCRYPT_KEY", "encrypt-key")
	t.Setenv("FEISHU_APP_EVENT_CALLBACK_PATH", "/callbacks/feishu/events")
	t.Setenv("FEISHU_APP_CARD_CALLBACK_PATH", "/callbacks/feishu/cards")

	cfg, err := ParseConfig(nil)
	if err != nil {
		t.Fatalf("ParseConfig failed: %v", err)
	}

	if !cfg.FeishuApp.Enabled {
		t.Fatal("expected feishu app to be enabled")
	}
	if cfg.FeishuApp.AppID != "cli_app_id" {
		t.Fatalf("expected feishu app id, got %s", cfg.FeishuApp.AppID)
	}
	if cfg.FeishuApp.EventCallbackPath != "/callbacks/feishu/events" {
		t.Fatalf("expected feishu event callback path, got %s", cfg.FeishuApp.EventCallbackPath)
	}
	if cfg.FeishuBot.Enabled {
		t.Fatal("expected feishu bot to stay disabled")
	}
}

func TestParseConfigAllowsFeishuBotOnlyMode(t *testing.T) {
	t.Setenv("GATEWAY_HTTP_PORT", ":8089")
	t.Setenv("GATEWAY_REDIS_ADDR", "redis.internal:6379")
	t.Setenv("FEISHU_BOT_ENABLED", "true")
	t.Setenv("FEISHU_BOT_WEBHOOK_URL", "https://open.feishu.cn/open-apis/bot/v2/hook/demo")
	t.Setenv("FEISHU_BOT_SECRET", "bot-secret")

	cfg, err := ParseConfig(nil)
	if err != nil {
		t.Fatalf("ParseConfig failed: %v", err)
	}

	if !cfg.FeishuBot.Enabled {
		t.Fatal("expected feishu bot to be enabled")
	}
	if cfg.FeishuBot.WebhookURL == "" {
		t.Fatal("expected feishu bot webhook url")
	}
	if cfg.FeishuApp.Enabled {
		t.Fatal("expected feishu app to stay disabled")
	}
}

func TestParseConfigRejectsNoEnabledAdapter(t *testing.T) {
	t.Setenv("GATEWAY_HTTP_PORT", ":8089")
	t.Setenv("GATEWAY_REDIS_ADDR", "redis.internal:6379")

	if _, err := ParseConfig(nil); err == nil {
		t.Fatal("expected ParseConfig to reject empty adapter config")
	}
}
