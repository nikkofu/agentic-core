package llm

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

// ValidateChatCompletionRequest 严格校验 OpenAI 请求
func ValidateChatCompletionRequest(raw []byte) (*ChatCompletionRequest, error) {
	var req ChatCompletionRequest
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields() // 开启严格模式：禁止未知字段

	if err := decoder.Decode(&req); err != nil {
		return nil, fmt.Errorf("invalid_request_error: %v", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return nil, errors.New("invalid_request_error: trailing json data")
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

	toolNames, err := validateTools(req.Tools)
	if err != nil {
		return nil, err
	}
	if err := validateToolChoice(req.ToolChoice, toolNames); err != nil {
		return nil, err
	}

	return &req, nil
}

type requestTool struct {
	Type     string `json:"type"`
	Function struct {
		Name string `json:"name"`
	} `json:"function"`
}

type requestToolChoice struct {
	Type     string `json:"type"`
	Function struct {
		Name string `json:"name"`
	} `json:"function"`
}

func validateTools(raw json.RawMessage) (map[string]struct{}, error) {
	if len(raw) == 0 {
		return nil, nil
	}

	var tools []requestTool
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&tools); err != nil {
		return nil, fmt.Errorf("invalid_request_error: invalid tools: %v", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return nil, errors.New("invalid_request_error: invalid tools: trailing json data")
	}

	names := make(map[string]struct{}, len(tools))
	for _, tool := range tools {
		if tool.Type != "function" {
			return nil, errors.New("invalid_request_error: tools only support function type")
		}
		if tool.Function.Name == "" {
			return nil, errors.New("invalid_request_error: tool function.name is required")
		}
		names[tool.Function.Name] = struct{}{}
	}

	return names, nil
}

func validateToolChoice(raw json.RawMessage, toolNames map[string]struct{}) error {
	if len(raw) == 0 {
		return nil
	}

	var stringChoice string
	if err := json.Unmarshal(raw, &stringChoice); err == nil {
		switch stringChoice {
		case "auto", "none", "required":
			if stringChoice == "required" && len(toolNames) == 0 {
				return errors.New("invalid_request_error: tool_choice required needs tools")
			}
			return nil
		default:
			return errors.New("invalid_request_error: invalid tool_choice")
		}
	}

	var objectChoice requestToolChoice
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&objectChoice); err != nil {
		return fmt.Errorf("invalid_request_error: invalid tool_choice: %v", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return errors.New("invalid_request_error: invalid tool_choice: trailing json data")
	}
	if objectChoice.Type != "function" {
		return errors.New("invalid_request_error: tool_choice object must have type=function")
	}
	if objectChoice.Function.Name == "" {
		return errors.New("invalid_request_error: tool_choice function.name is required")
	}
	if len(toolNames) == 0 {
		return errors.New("invalid_request_error: tool_choice function requires tools")
	}
	if _, ok := toolNames[objectChoice.Function.Name]; !ok {
		return errors.New("invalid_request_error: tool_choice function not found in tools")
	}
	return nil
}

// MapProviderError 将上游 Provider 错误映射为 OpenAI 兼容的错误
func MapProviderError(err error) (status int, typ string, msg string) {
	if err == nil {
		return 200, "", ""
	}

	errStr := err.Error()
	switch {
	case strings.Contains(errStr, "invalid_request_error"):
		return 400, "invalid_request_error", errStr
	case strings.Contains(errStr, "approval rejected"):
		return 403, "permission_error", errStr
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
