package skill

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// CurrentTimeSkill 是一个内置技能，用于获取当前时间
type CurrentTimeSkill struct{}

func (s *CurrentTimeSkill) Name() string {
	return "get_current_time"
}

func (s *CurrentTimeSkill) Description() string {
	return "Returns the current server time."
}

func (s *CurrentTimeSkill) Schema() string {
	return `{"type": "object", "properties": {}}`
}

func (s *CurrentTimeSkill) IsWriteOperation() bool {
	return false
}

func (s *CurrentTimeSkill) Execute(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
	now := time.Now().Format(time.RFC3339)
	res := map[string]string{"time": now}
	return json.Marshal(res)
}

// HttpGetSkill 是一个简单的网络请求技能
type HttpGetSkill struct{}

func (s *HttpGetSkill) Name() string {
	return "http_get"
}

func (s *HttpGetSkill) Description() string {
	return "Fetches the content of a given URL."
}

func (s *HttpGetSkill) Schema() string {
	return `{
		"type": "object",
		"properties": {
			"url": {"type": "string", "description": "The URL to fetch"}
		},
		"required": ["url"]
	}`
}

func (s *HttpGetSkill) IsWriteOperation() bool {
	return false // 假设 GET 不会修改外部状态
}

func (s *HttpGetSkill) Execute(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
	var input struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(params, &input); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}

	// 演示目的，这里直接返回模拟数据。未来这里可以真正发起请求或通过 Wasm 沙盒发起。
	mockResp := fmt.Sprintf("Mock response from %s: <html>...</html>", input.URL)
	res := map[string]string{"status": "200", "body": mockResp}
	return json.Marshal(res)
}
