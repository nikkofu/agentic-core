package skill

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	HeaderSignature = "X-Signature"
	HeaderTimestamp = "X-Timestamp"
	HeaderNonce     = "X-Nonce"

	nonceTTL   = 300 * time.Second
	timeWindow = 300 * time.Second
	sigPrefix  = "sha256="
)

// NonceStore 负责存储和校验 nonce，防止重放攻击
type NonceStore interface {
	CheckAndSet(nonce string, ttl time.Duration) bool
}

type InMemNonceStore struct {
	mu     sync.Mutex
	nonces map[string]time.Time
}

func NewInMemNonceStore() *InMemNonceStore {
	return &InMemNonceStore{nonces: make(map[string]time.Time)}
}

func (s *InMemNonceStore) CheckAndSet(nonce string, ttl time.Duration) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for k, v := range s.nonces {
		if now.After(v) {
			delete(s.nonces, k)
		}
	}

	if _, ok := s.nonces[nonce]; ok {
		return false
	}

	s.nonces[nonce] = now.Add(ttl)
	return true
}

// GenerateSignature 生成 Webhook 签名 (用于测试或发送端)
func GenerateSignature(secret string, timestamp int64, nonce string, body []byte) string {
	payload := fmt.Sprintf("%d.%s.%s", timestamp, nonce, string(body))
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(payload))
	return sigPrefix + hex.EncodeToString(h.Sum(nil))
}

// VerifyWebhookSignature 验证 Webhook 签名
func VerifyWebhookSignature(headers http.Header, body []byte, secret string, store NonceStore, now time.Time) error {
	sig := headers.Get(HeaderSignature)
	tsStr := headers.Get(HeaderTimestamp)
	nonce := headers.Get(HeaderNonce)

	if sig == "" || tsStr == "" || nonce == "" {
		return fmt.Errorf("missing security headers")
	}
	if store == nil {
		return fmt.Errorf("nonce store required")
	}

	ts, err := strconv.ParseInt(tsStr, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid timestamp")
	}
	if diff := now.Unix() - ts; diff < -int64(timeWindow.Seconds()) || diff > int64(timeWindow.Seconds()) {
		return fmt.Errorf("timestamp out of window")
	}

	if !strings.HasPrefix(sig, sigPrefix) {
		return fmt.Errorf("invalid signature prefix")
	}
	received, err := hex.DecodeString(strings.TrimPrefix(sig, sigPrefix))
	if err != nil {
		return fmt.Errorf("invalid signature digest")
	}

	expectedPayload := fmt.Sprintf("%d.%s.%s", ts, nonce, string(body))
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(expectedPayload))
	expected := h.Sum(nil)
	if !hmac.Equal(received, expected) {
		return fmt.Errorf("invalid signature")
	}

	if ok := store.CheckAndSet(nonce, nonceTTL); !ok {
		return fmt.Errorf("replayed nonce")
	}

	return nil
}
