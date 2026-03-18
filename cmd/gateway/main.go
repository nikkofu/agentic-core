package main

import (
	"agentic-core/internal/bus"
	gw "agentic-core/internal/gateway"
	"agentic-core/internal/gateway/feishu"
	"agentic-core/internal/gateway/wecom"
	"agentic-core/internal/gateway/wecomrobot"
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/redis/go-redis/v9"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "gateway error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := gw.ParseConfig(os.Args[1:])
	if err != nil {
		return err
	}

	ctx := context.Background()
	var transport *bus.RedisTransport
	if cfg.RedisAddr == "skip" {
		return fmt.Errorf("gateway requires redis transport in runtime")
	}

	rdb := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr})
	if err := rdb.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("redis connection failed: %w", err)
	}
	transport = bus.NewRedisTransport(rdb)

	router := gw.NewSessionRouter(transport)
	mux, err := buildGatewayMux(cfg, router)
	if err != nil {
		return err
	}

	if cfg.WeCom.Enabled() && cfg.WeCom.MediaRetentionDays > 0 {
		wecom.StartMediaRetention(ctx, wecom.MediaRetentionConfig{
			MediaDir:  cfg.WeCom.MediaDir,
			Retention: time.Duration(cfg.WeCom.MediaRetentionDays) * 24 * time.Hour,
		})
	}

	if err := router.StartStreamListener(ctx); err != nil {
		return err
	}

	server := &http.Server{
		Addr:    cfg.HTTPPort,
		Handler: mux,
	}
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func buildGatewayMux(cfg gw.Config, router *gw.SessionRouter) (*http.ServeMux, error) {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})
	mux.Handle("/v1/channels/incoming", gw.NewIngressHandler(router, gw.IngressHandlerConfig{
		Secret: cfg.IngressSecret,
	}))

	if cfg.WeCom.Enabled() {
		adapter, err := wecom.NewAdapter(wecom.Config{
			CorpID:         cfg.WeCom.CorpID,
			AgentID:        cfg.WeCom.AgentID,
			CorpSecret:     cfg.WeCom.CorpSecret,
			Token:          cfg.WeCom.Token,
			EncodingAESKey: cfg.WeCom.EncodingAESKey,
			CallbackPath:   cfg.WeCom.CallbackPath,
			APIBaseURL:     cfg.WeCom.APIBaseURL,
			MediaDir:       cfg.WeCom.MediaDir,
		}, nil)
		if err != nil {
			return nil, err
		}
		router.RegisterAdapter(adapter)
		mux.Handle(cfg.WeCom.CallbackPath, adapter.CallbackHandler(router))
	}

	if cfg.WeComRobot.Enabled() {
		router.RegisterAdapter(wecomrobot.NewAdapter(wecomrobot.Config{
			WebhookURL: cfg.WeComRobot.WebhookURL,
		}, nil))
	}

	if cfg.FeishuApp.IsEnabled() {
		adapter, err := feishu.NewAppAdapter(feishu.AppConfig{
			AppID:             cfg.FeishuApp.AppID,
			AppSecret:         cfg.FeishuApp.AppSecret,
			VerificationToken: cfg.FeishuApp.VerificationToken,
			EncryptKey:        cfg.FeishuApp.EncryptKey,
			EventCallbackPath: cfg.FeishuApp.EventCallbackPath,
			CardCallbackPath:  cfg.FeishuApp.CardCallbackPath,
			APIBaseURL:        cfg.FeishuApp.APIBaseURL,
		}, nil)
		if err != nil {
			return nil, err
		}
		router.RegisterAdapter(adapter)
		mux.Handle(cfg.FeishuApp.EventCallbackPath, adapter.EventHandler(router))
		mux.Handle(cfg.FeishuApp.CardCallbackPath, adapter.CardHandler(router))
	}

	if cfg.FeishuBot.IsEnabled() {
		router.RegisterAdapter(feishu.NewBotAdapter(feishu.BotConfig{
			WebhookURL: cfg.FeishuBot.WebhookURL,
			Secret:     cfg.FeishuBot.Secret,
		}, nil))
	}

	return mux, nil
}
