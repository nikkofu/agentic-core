package main

import (
	"agentic-core/internal/bus"
	gw "agentic-core/internal/gateway"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGatewayCanInitializeWithFeishuOnlyConfig(t *testing.T) {
	router := gw.NewSessionRouter(bus.NewFakeTransport())
	mux, err := buildGatewayMux(gw.Config{
		HTTPPort:  ":8081",
		RedisAddr: "redis.internal:6379",
		FeishuApp: gw.FeishuAppConfig{
			Enabled:           true,
			AppID:             "cli_app_id",
			AppSecret:         "cli_secret",
			VerificationToken: "verify-token",
			EncryptKey:        "encrypt-key",
			EventCallbackPath: "/callbacks/feishu/events",
			CardCallbackPath:  "/callbacks/feishu/cards",
		},
	}, router)
	if err != nil {
		t.Fatalf("buildGatewayMux failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestGatewayRegistersFeishuHandlersWhenEnabled(t *testing.T) {
	router := gw.NewSessionRouter(bus.NewFakeTransport())
	mux, err := buildGatewayMux(gw.Config{
		HTTPPort:  ":8081",
		RedisAddr: "redis.internal:6379",
		FeishuApp: gw.FeishuAppConfig{
			Enabled:           true,
			AppID:             "cli_app_id",
			AppSecret:         "cli_secret",
			VerificationToken: "verify-token",
			EncryptKey:        "encrypt-key",
			EventCallbackPath: "/callbacks/feishu/events",
			CardCallbackPath:  "/callbacks/feishu/cards",
		},
	}, router)
	if err != nil {
		t.Fatalf("buildGatewayMux failed: %v", err)
	}

	eventReq := httptest.NewRequest(http.MethodPost, "/callbacks/feishu/events", strings.NewReader(`{"challenge":"hello","token":"verify-token","type":"url_verification"}`))
	eventRec := httptest.NewRecorder()
	mux.ServeHTTP(eventRec, eventReq)
	if eventRec.Code != http.StatusOK {
		t.Fatalf("expected event handler 200, got %d body=%s", eventRec.Code, eventRec.Body.String())
	}

	cardReq := httptest.NewRequest(http.MethodPost, "/callbacks/feishu/cards", strings.NewReader(`{"challenge":"hello-card","token":"verify-token","type":"url_verification"}`))
	cardRec := httptest.NewRecorder()
	mux.ServeHTTP(cardRec, cardReq)
	if cardRec.Code != http.StatusOK {
		t.Fatalf("expected card handler 200, got %d body=%s", cardRec.Code, cardRec.Body.String())
	}
}

func TestGatewayCanInitializeWithDingTalkOnlyConfig(t *testing.T) {
	router := gw.NewSessionRouter(bus.NewFakeTransport())
	mux, err := buildGatewayMux(gw.Config{
		HTTPPort:  ":8081",
		RedisAddr: "redis.internal:6379",
		DingTalkApp: gw.DingTalkAppConfig{
			Enabled:           true,
			ClientID:          "ding-app-id",
			ClientSecret:      "ding-secret",
			AgentID:           900001,
			Token:             "ding-token",
			AESKey:            "ding-aes-key",
			EventCallbackPath: "/callbacks/dingtalk/events",
			CardCallbackPath:  "/callbacks/dingtalk/cards",
		},
	}, router)
	if err != nil {
		t.Fatalf("buildGatewayMux failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestGatewayRegistersDingTalkHandlersWhenEnabled(t *testing.T) {
	router := gw.NewSessionRouter(bus.NewFakeTransport())
	mux, err := buildGatewayMux(gw.Config{
		HTTPPort:  ":8081",
		RedisAddr: "redis.internal:6379",
		DingTalkApp: gw.DingTalkAppConfig{
			Enabled:           true,
			ClientID:          "ding-app-id",
			ClientSecret:      "ding-secret",
			AgentID:           900001,
			Token:             "ding-token",
			AESKey:            "ding-aes-key",
			EventCallbackPath: "/callbacks/dingtalk/events",
			CardCallbackPath:  "/callbacks/dingtalk/cards",
		},
	}, router)
	if err != nil {
		t.Fatalf("buildGatewayMux failed: %v", err)
	}

	eventReq := httptest.NewRequest(http.MethodPost, "/callbacks/dingtalk/events", strings.NewReader(`{"challenge":"hello","encrypt":"payload"}`))
	eventRec := httptest.NewRecorder()
	mux.ServeHTTP(eventRec, eventReq)
	if eventRec.Code == http.StatusNotFound {
		t.Fatalf("expected dingtalk event handler to be registered, got %d", eventRec.Code)
	}

	cardReq := httptest.NewRequest(http.MethodPost, "/callbacks/dingtalk/cards", strings.NewReader(`{"challenge":"hello-card","encrypt":"payload"}`))
	cardRec := httptest.NewRecorder()
	mux.ServeHTTP(cardRec, cardReq)
	if cardRec.Code == http.StatusNotFound {
		t.Fatalf("expected dingtalk card handler to be registered, got %d", cardRec.Code)
	}
}
