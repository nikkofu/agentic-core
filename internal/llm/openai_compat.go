package llm

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// ValidateChatCompletionRequest 严格校验 OpenAI 请求
func ValidateChatCompletionRequest(raw []byte) (*ChatCompletionRequest, error) {
	var req ChatCompletionRequest
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields() // 开启严格模式：禁止未知字段

	if err := decoder.Decode(&req); err != nil {
		return nil, fmt.Errorf("invalid_request_error: %v", err)
	}

	// 校验必填字段
	if req.Model == "" {
		return nil, errors.New("invalid_request_error: missing model")
	}
	if len(req.Messages) == 0 {
		return nil, errors.New("invalid_request_error: messages cannot be empty")
	}

	// 校验消息角色
	validRoles := map[string]bool{"system": true, "user": true, "assistant": true, "tool": true}
	for _, msg := range req.Messages {
		if !validRoles[msg.Role] {
			return nil, fmt.Errorf("invalid_request_error: invalid message role: %s", msg.Role)
		}
	}

	// 校验 temperature [0, 2]
	if req.Temperature != nil {
		if *req.Temperature < 0 || *req.Temperature > 2 {
			return nil, errors.New("invalid_request_error: temperature must be between 0 and 2")
		}
	}

	return &req, nil
}

// MapProviderError 将上游 Provider 错误映射为 OpenAI 兼容的错误
func MapProviderError(err error) (status int, typ string, msg string) {
	if err == nil {
		return 200, "", ""
	}

	errStr := err.Error()
	switch {
	case strings.Contains(errStr, "401") || strings.Contains(errStr, "unauthorized") || strings.Contains(errStr, "invalid_api_key"):
		return 401, "authentication_error", errStr
	case strings.Contains(errStr, "429") || strings.Contains(errStr, "rate_limit"):
		return 429, "rate_limit_error", errStr
	case strings.Contains(errStr, "context deadline exceeded") || strings.Contains(errStr, "timeout"):
		return 504, "timeout_error", errStr
	default:
		return 502, "api_error", errStr
	}
}
