package llm

import (
	"agentic-core/internal/session"
	"context"
	"fmt"
)

// ContextGuard 负责管理 LLM 的上下文窗口大小
type ContextGuard struct {
	Policy session.CompactorPolicy
}

func NewContextGuard(maxTokens int) *ContextGuard {
	if maxTokens <= 0 {
		maxTokens = 4096 // 默认值
	}
	return &ContextGuard{
		Policy: session.CompactorPolicy{
			MaxTokens:  maxTokens,
			KeepRecent: 6, // 默认保留最近 6 轮
		},
	}
}

// SemanticCompact 语义化压缩历史
func (c *ContextGuard) SemanticCompact(ctx context.Context, p Provider, systemPrompt string, history []session.ChatMessage, newQuery string) ([]ChatMessage, bool) {
	toSum, kept := session.CompactHistory(history, c.Policy)
	
	var messages []ChatMessage
	messages = append(messages, ChatMessage{Role: "system", Content: systemPrompt})

	usedSummary := false
	if len(toSum) > 0 && p != nil {
		summary, err := c.summarize(ctx, p, toSum)
		if err == nil {
			messages = append(messages, ChatMessage{
				Role:    "system",
				Content: fmt.Sprintf("[Previous Context Summary]: %s", summary),
			})
			usedSummary = true
		}
	}

	for _, h := range kept {
		messages = append(messages, ChatMessage{
			Role:    h.Role,
			Content: h.Content,
		})
	}

	messages = append(messages, ChatMessage{
		Role:    "user",
		Content: newQuery,
	})

	return messages, usedSummary
}

func (c *ContextGuard) summarize(ctx context.Context, p Provider, msgs []session.ChatMessage) (string, error) {
	text := session.FormatForSummary(msgs)
	prompt := "Summarize the following conversation history briefly while preserving key facts and decisions:\n\n" + text

	req := InferenceRequest{
		Messages: []ChatMessage{
			{Role: "user", Content: prompt},
		},
	}
	return p.Predict(ctx, req)
}

// PromptBuilder 用于组装发送给 LLM 的消息
type PromptBuilder struct {
	Guard *ContextGuard
}

func NewPromptBuilder(maxTokens int) *PromptBuilder {
	return &PromptBuilder{Guard: NewContextGuard(maxTokens)}
}

// BuildMessages 组装 OpenAI 兼容的消息列表格式 (带自动压缩)
func (pb *PromptBuilder) BuildMessages(ctx context.Context, p Provider, systemPrompt string, history []session.ChatMessage, newQuery string) []ChatMessage {
	messages, _ := pb.Guard.SemanticCompact(ctx, p, systemPrompt, history, newQuery)
	return messages
}
