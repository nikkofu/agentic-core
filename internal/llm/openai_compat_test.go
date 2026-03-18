package llm

import (
	"testing"
)

func TestValidateChatCompletionRequest(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		errType string
	}{
		{
			name:    "valid request",
			input:   `{"model": "gpt-4", "messages": [{"role": "user", "content": "hello"}]}`,
			wantErr: false,
		},
		{
			name:    "missing model",
			input:   `{"messages": [{"role": "user", "content": "hello"}]}`,
			wantErr: true,
			errType: "invalid_request_error",
		},
		{
			name:    "empty messages",
			input:   `{"model": "gpt-4", "messages": []}`,
			wantErr: true,
			errType: "invalid_request_error",
		},
		{
			name:    "invalid role",
			input:   `{"model": "gpt-4", "messages": [{"role": "god", "content": "hello"}]}`,
			wantErr: true,
			errType: "invalid_request_error",
		},
		{
			name:    "unknown field strict mode",
			input:   `{"model": "gpt-4", "messages": [{"role": "user", "content": "hello"}], "unknown_field": "value"}`,
			wantErr: true,
			errType: "invalid_request_error",
		},
		{
			name:    "invalid temperature",
			input:   `{"model": "gpt-4", "messages": [{"role": "user", "content": "hello"}], "temperature": 2.5}`,
			wantErr: true,
			errType: "invalid_request_error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ValidateChatCompletionRequest([]byte(tt.input))
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateChatCompletionRequest() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestMapProviderError(t *testing.T) {
	// 暂不测试具体映射，等待实现后补充逻辑
}
