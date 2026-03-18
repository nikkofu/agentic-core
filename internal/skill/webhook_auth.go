package skill

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"
)

const (
	HeaderSignature = "X-Agentic-Signature"
	HeaderTimestamp = "X-Agentic-Timestamp"
	HeaderNonce     = "X-Agentic-Nonce"
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

	// 清理过期 nonce (简单处理，实际应用中应有更高效的清理机制)
	now := time.Now()
	for k, v := range s.nonces {
		if now.After(v) {
			delete(s.nonces, k)
		}
	}

	if _, ok := s.nonces[nonce]; ok {
		return false // 已存在，重放
	}

	s.nonces[nonce] = now.Add(ttl)
	return true
}

// GenerateSignature 生成 Webhook 签名 (用于测试或发送端)
func GenerateSignature(secret string, timestamp int64, nonce string, body []byte) string {
	payload := fmt.Sprintf("%d.%s.%s", timestamp, nonce, string(body))
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(payload))
	return hex.EncodeToString(h.Sum(nil))
}

// VerifyWebhookSignature 验证 Webhook 签名
func VerifyWebhookSignature(headers http.Header, body []byte, secret string, store NonceStore, now time.Time) error {
	sig := headers.Get(HeaderSignature)
	tsStr := headers.Get(HeaderTimestamp)
	nonce := headers.Get(HeaderNonce)

	if sig == "" || tsStr == "" || nonce == "" {
		return fmt.Errorf("missing security headers")
	}

	// 1. 验证时间戳窗口 (允许前后 5 分钟误差)
	ts, err := strconv.ParseInt(tsStr, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid timestamp")
	}
	diff := now.Unix() - ts
	if diff < -300 || diff > 300 {
		return fmt.Errorf("timestamp out of window")
	}

	// 2. 验证 Nonce 重放
	if store != nil {
		if ok := store.CheckAndSet(nonce, 10*time.Minute); !ok {
			return fmt.Errorf("replayed nonce")
		}
	}

	// 3. 验证 HMAC
	expected := GenerateSignature(secret, ts, nonce, body)
	if !hmac.Equal([]byte(sig), []byte(expected)) {
		return fmt.Errorf("invalid signature")
	}

	return nil
}
