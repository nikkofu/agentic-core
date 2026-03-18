package main

import (
	"encoding/xml"
	"net/url"
	"testing"

	"agentic-core/internal/gateway/wecom"
)

func TestBuildCallbackRequestProducesDecryptableEnvelope(t *testing.T) {
	cfg := requestConfig{
		Endpoint:       "http://127.0.0.1:8081/callbacks/wecom",
		CorpID:         "ww-test-corp",
		AgentID:        1000002,
		Token:          "gateway-token",
		EncodingAESKey: "abcdefghijklmnopqrstuvwxyz0123456789ABCDEFG",
		Timestamp:      "1773811200",
		Nonce:          "nonce-sim-001",
		InnerXML: `<xml>
<ToUserName><![CDATA[ww-test-corp]]></ToUserName>
<FromUserName><![CDATA[zhangsan]]></FromUserName>
<CreateTime>1773811200</CreateTime>
<MsgType><![CDATA[text]]></MsgType>
<Content><![CDATA[hello simulator]]></Content>
<MsgId>1001</MsgId>
<AgentID>1000002</AgentID>
</xml>`,
	}

	targetURL, body, err := buildCallbackRequest(cfg)
	if err != nil {
		t.Fatalf("buildCallbackRequest failed: %v", err)
	}

	parsedURL, err := url.Parse(targetURL)
	if err != nil {
		t.Fatalf("parse target url failed: %v", err)
	}
	if parsedURL.Path != "/callbacks/wecom" {
		t.Fatalf("expected callbacks path, got %s", parsedURL.Path)
	}
	if parsedURL.Query().Get("timestamp") != cfg.Timestamp {
		t.Fatalf("expected timestamp query %s, got %s", cfg.Timestamp, parsedURL.Query().Get("timestamp"))
	}
	if parsedURL.Query().Get("nonce") != cfg.Nonce {
		t.Fatalf("expected nonce query %s, got %s", cfg.Nonce, parsedURL.Query().Get("nonce"))
	}

	var envelope struct {
		XMLName xml.Name `xml:"xml"`
		Encrypt string   `xml:"Encrypt"`
		AgentID int64    `xml:"AgentID"`
	}
	if err := xml.Unmarshal(body, &envelope); err != nil {
		t.Fatalf("unmarshal callback body failed: %v", err)
	}
	if envelope.AgentID != cfg.AgentID {
		t.Fatalf("expected agent id %d, got %d", cfg.AgentID, envelope.AgentID)
	}
	if envelope.Encrypt == "" {
		t.Fatal("expected encrypted payload present")
	}

	codec, err := wecom.NewCodec(wecom.Config{
		CorpID:         cfg.CorpID,
		Token:          cfg.Token,
		EncodingAESKey: cfg.EncodingAESKey,
	})
	if err != nil {
		t.Fatalf("NewCodec failed: %v", err)
	}
	plain, err := codec.Decrypt(parsedURL.Query().Get("msg_signature"), cfg.Timestamp, cfg.Nonce, envelope.Encrypt)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}
	if string(plain) != cfg.InnerXML {
		t.Fatalf("expected decrypted inner xml preserved, got %s", string(plain))
	}
}
