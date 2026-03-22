package skill

import (
	"fmt"
	"net/http"
	"testing"
	"time"
)

func TestVerifyWebhookSignature(t *testing.T) {
	secret := "test-secret"
	body := []byte(`{"task_id": "123", "approved": true}`)
	now := time.Unix(1_700_000_000, 0)
	ts := now.Unix()
	nonce := "unique-nonce-1"

	t.Run("missing headers", func(t *testing.T) {
		err := VerifyWebhookSignature(http.Header{}, body, secret, nil, now)
		if err == nil || err.Error() != "missing security headers" {
			t.Fatalf("expected missing headers error, got: %v", err)
		}
	})

	t.Run("invalid timestamp format", func(t *testing.T) {
		headers := http.Header{}
		headers.Set(HeaderSignature, GenerateSignature(secret, ts, nonce, body))
		headers.Set(HeaderTimestamp, "not-a-timestamp")
		headers.Set(HeaderNonce, nonce)

		err := VerifyWebhookSignature(headers, body, secret, nil, now)
		if err == nil || err.Error() != "invalid timestamp" {
			t.Fatalf("expected invalid timestamp error, got: %v", err)
		}
	})

	t.Run("timestamp out of window", func(t *testing.T) {
		oldTS := now.Add(-301 * time.Second).Unix()
		headers := http.Header{}
		headers.Set(HeaderSignature, GenerateSignature(secret, oldTS, nonce, body))
		headers.Set(HeaderTimestamp, fmt.Sprintf("%d", oldTS))
		headers.Set(HeaderNonce, nonce)

		err := VerifyWebhookSignature(headers, body, secret, nil, now)
		if err == nil || err.Error() != "timestamp out of window" {
			t.Fatalf("expected timestamp window error, got: %v", err)
		}
	})

	t.Run("invalid signature prefix", func(t *testing.T) {
		headers := http.Header{}
		headers.Set(HeaderSignature, "hmac=deadbeef")
		headers.Set(HeaderTimestamp, fmt.Sprintf("%d", ts))
		headers.Set(HeaderNonce, nonce)

		err := VerifyWebhookSignature(headers, body, secret, nil, now)
		if err == nil || err.Error() != "invalid signature prefix" {
			t.Fatalf("expected prefix error, got: %v", err)
		}
	})

	t.Run("invalid signature digest", func(t *testing.T) {
		headers := http.Header{}
		headers.Set(HeaderSignature, "sha256=zzzz")
		headers.Set(HeaderTimestamp, fmt.Sprintf("%d", ts))
		headers.Set(HeaderNonce, nonce)

		err := VerifyWebhookSignature(headers, body, secret, nil, now)
		if err == nil || err.Error() != "invalid signature digest" {
			t.Fatalf("expected digest error, got: %v", err)
		}
	})

	t.Run("replayed nonce", func(t *testing.T) {
		store := NewInMemNonceStore()
		headers := http.Header{}
		headers.Set(HeaderSignature, GenerateSignature(secret, ts, nonce, body))
		headers.Set(HeaderTimestamp, fmt.Sprintf("%d", ts))
		headers.Set(HeaderNonce, nonce)

		if err := VerifyWebhookSignature(headers, body, secret, store, now); err != nil {
			t.Fatalf("first verification failed: %v", err)
		}
		if err := VerifyWebhookSignature(headers, body, secret, store, now); err == nil || err.Error() != "replayed nonce" {
			t.Fatalf("expected replayed nonce error, got: %v", err)
		}
	})

	t.Run("valid canonical string verification", func(t *testing.T) {
		store := NewInMemNonceStore()
		headers := http.Header{}
		headers.Set(HeaderSignature, GenerateSignature(secret, ts, nonce, body))
		headers.Set(HeaderTimestamp, fmt.Sprintf("%d", ts))
		headers.Set(HeaderNonce, nonce)

		if err := VerifyWebhookSignature(headers, body, secret, store, now); err != nil {
			t.Fatalf("expected success, got: %v", err)
		}
	})
}
