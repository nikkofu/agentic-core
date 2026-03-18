package llm

import (
	"testing"
)

func TestResolveByAlias(t *testing.T) {
	resolver := NewModelResolver()
	
	// 模拟注册 Provider
	mockProvider := &OpenAIProvider{} // 此时仅为类型占位
	resolver.Register("openai", mockProvider)

	// 注册路由
	route := StaticRoute{
		Alias:         "gpt-4-stable",
		Provider:      "openai",
		UpstreamModel: "gpt-4-0613",
	}
	resolver.RegisterRoute(route)

	tests := []struct {
		name      string
		alias     string
		wantModel string
		wantErr   bool
	}{
		{
			name:      "resolve valid alias",
			alias:     "gpt-4-stable",
			wantModel: "gpt-4-0613",
			wantErr:   false,
		},
		{
			name:      "resolve missing alias",
			alias:     "unknown-model",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, r, err := resolver.ResolveByAlias(tt.alias)
			if (err != nil) != tt.wantErr {
				t.Errorf("ResolveByAlias() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if p == nil {
					t.Error("ResolveByAlias() returned nil provider")
				}
				if r.UpstreamModel != tt.wantModel {
					t.Errorf("ResolveByAlias() got upstream model = %v, want %v", r.UpstreamModel, tt.wantModel)
				}
			}
		})
	}
}
