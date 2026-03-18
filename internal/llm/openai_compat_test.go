package llm

import (
	"errors"
	"strings"
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
		{
			name:    "valid tools with object tool_choice",
			input:   `{"model":"gpt-4","messages":[{"role":"user","content":"hello"}],"tools":[{"type":"function","function":{"name":"lookup"}}],"tool_choice":{"type":"function","function":{"name":"lookup"}}}`,
			wantErr: false,
		},
		{
			name:    "rejects non-function tool",
			input:   `{"model":"gpt-4","messages":[{"role":"user","content":"hello"}],"tools":[{"type":"code_interpreter","function":{"name":"lookup"}}]}`,
			wantErr: true,
			errType: "invalid_request_error",
		},
		{
			name:    "rejects invalid tool_choice string",
			input:   `{"model":"gpt-4","messages":[{"role":"user","content":"hello"}],"tool_choice":"sometimes"}`,
			wantErr: true,
			errType: "invalid_request_error",
		},
		{
			name:    "rejects tool_choice function not found in tools",
			input:   `{"model":"gpt-4","messages":[{"role":"user","content":"hello"}],"tools":[{"type":"function","function":{"name":"lookup"}}],"tool_choice":{"type":"function","function":{"name":"missing"}}}`,
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
			if tt.wantErr && err != nil && tt.errType != "" && !strings.Contains(err.Error(), tt.errType) {
				t.Errorf("expected error type %q, got %v", tt.errType, err)
			}
		})
	}
}

func TestMapProviderError(t *testing.T) {
	status, typ, _ := MapProviderError(errors.New("invalid_request_error: bad model"))
	if status != 400 || typ != "invalid_request_error" {
		t.Fatalf("expected 400 invalid_request_error, got %d %s", status, typ)
	}
}
