package llm

import (
	"context"
	"io"
)

// Provider 定义了 LLM 处理器的标准接口
type Provider interface {
	// Predict 接收内部推理请求并返回最终结果（非流式）
	Predict(ctx context.Context, req InferenceRequest) (string, error)
	// PredictStream 为流式输出提供支持
	PredictStream(ctx context.Context, req InferenceRequest) (io.ReadCloser, error)
}

// ModelConfig 模型配置
type ModelConfig struct {
	ProviderName string  `json:"provider_name"` // openai, claude, ollama
	ModelID      string  `json:"model_id"`
	APIKey       string  `json:"api_key"`
	BaseURL      string  `json:"base_url"`
	Temperature  float32 `json:"temperature"`
}
