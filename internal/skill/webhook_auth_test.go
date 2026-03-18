package skill

import (
	"fmt"
	"net/http"
	"testing"
	"time"
)

func TestWebhookAuth(t *testing.T) {
	secret := "test-secret"
	body := []byte(`{"task_id": "123", "approved": true}`)
	now := time.Now()
	nonce := "unique-nonce-1"

	t.Run("accepts valid signature", func(t *testing.T) {
		ts := now.Unix()
		sig := GenerateSignature(secret, ts, nonce, body)
		
		headers := http.Header{}
		headers.Set(HeaderSignature, sig)
		headers.Set(HeaderTimestamp, fmt.Sprintf("%d", ts))
		headers.Set(HeaderNonce, nonce)

		err := VerifyWebhookSignature(headers, body, secret, NewInMemNonceStore(), now)
		if err != nil {
			t.Errorf("expected success, got error: %v", err)
		}
	})

	t.Run("rejects expired timestamp", func(t *testing.T) {
		ts := now.Unix() - 600 // 10 分钟前
		sig := GenerateSignature(secret, ts, nonce, body)
		
		headers := http.Header{}
		headers.Set(HeaderSignature, sig)
		headers.Set(HeaderTimestamp, fmt.Sprintf("%d", ts))
		headers.Set(HeaderNonce, nonce)

		err := VerifyWebhookSignature(headers, body, secret, nil, now)
		if err == nil || err.Error() != "timestamp out of window" {
			t.Errorf("expected timestamp error, got: %v", err)
		}
	})

	t.Run("rejects invalid HMAC", func(t *testing.T) {
		ts := now.Unix()
		sig := "wrong-signature"
		
		headers := http.Header{}
		headers.Set(HeaderSignature, sig)
		headers.Set(HeaderTimestamp, fmt.Sprintf("%d", ts))
		headers.Set(HeaderNonce, nonce)

		err := VerifyWebhookSignature(headers, body, secret, nil, now)
		if err == nil || err.Error() != "invalid signature" {
			t.Errorf("expected signature error, got: %v", err)
		}
	})

	t.Run("rejects replayed nonce", func(t *testing.T) {
		store := NewInMemNonceStore()
		ts := now.Unix()
		sig := GenerateSignature(secret, ts, nonce, body)
		
		headers := http.Header{}
		headers.Set(HeaderSignature, sig)
		headers.Set(HeaderTimestamp, fmt.Sprintf("%d", ts))
		headers.Set(HeaderNonce, nonce)

		// 第一次成功
		err := VerifyWebhookSignature(headers, body, secret, store, now)
		if err != nil {
			t.Fatalf("first attempt failed: %v", err)
		}

		// 第二次重放失败
		err = VerifyWebhookSignature(headers, body, secret, store, now)
		if err == nil || err.Error() != "replayed nonce" {
			t.Errorf("expected replayed nonce error, got: %v", err)
		}
	})
}
