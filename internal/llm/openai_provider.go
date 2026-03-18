package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type OpenAIProvider struct {
	config ModelConfig
	client *http.Client
}

func NewOpenAIProvider(config ModelConfig) *OpenAIProvider {
	if config.BaseURL == "" {
		config.BaseURL = "https://api.openai.com/v1"
	}
	return &OpenAIProvider{
		config: config,
		client: &http.Client{},
	}
}

type chatCompletionResponse struct {
	Choices []struct {
		Message ChatMessage `json:"message"`
	} `json:"choices"`
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (p *OpenAIProvider) Predict(ctx context.Context, req InferenceRequest) (string, error) {
	reqBody := ChatCompletionRequest{
		Model:       p.config.ModelID,
		Messages:    req.Messages,
		Temperature: req.Temperature,
		Stream:      false,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	url := fmt.Sprintf("%s/chat/completions", p.config.BaseURL)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", err
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", p.config.APIKey))

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != http.StatusOK {
		var errorResponse chatCompletionResponse
		json.Unmarshal(body, &errorResponse)
		return "", fmt.Errorf("openai error (%d): %s", resp.StatusCode, errorResponse.Error.Message)
	}

	var completionResponse chatCompletionResponse
	if err := json.Unmarshal(body, &completionResponse); err != nil {
		return "", err
	}

	if len(completionResponse.Choices) == 0 {
		return "", fmt.Errorf("openai returned no choices")
	}

	return completionResponse.Choices[0].Message.Content, nil
}

func (p *OpenAIProvider) PredictStream(ctx context.Context, req InferenceRequest) (io.ReadCloser, error) {
	reqBody := ChatCompletionRequest{
		Model:       p.config.ModelID,
		Messages:    req.Messages,
		Temperature: req.Temperature,
		Stream:      true,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/chat/completions", p.config.BaseURL)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", p.config.APIKey))

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("openai streaming error (%d): %s", resp.StatusCode, string(body))
	}

	return resp.Body, nil
}
