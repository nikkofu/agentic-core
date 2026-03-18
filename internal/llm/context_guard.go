package llm

import (
	"agentic-core/internal/session"
	"context"
	"fmt"
	"strings"
)

// ContextGuard 负责管理 LLM 的上下文窗口大小
type ContextGuard struct {
	MaxTokens int // 假设最大 Token 数
}

func NewContextGuard(maxTokens int) *ContextGuard {
	if maxTokens <= 0 {
		maxTokens = 4096 // 默认值
	}
	return &ContextGuard{MaxTokens: maxTokens}
}

// EstimateTokens 粗略估算字符串的 Token 数量 (假设 1 token ≈ 4 chars)
func (c *ContextGuard) EstimateTokens(text string) int {
	return len(text) / 4
}

// Compact 接收系统提示词和历史消息，如果超过 MaxTokens，则丢弃最早的消息，
// 保证最终组装的 Prompt 不会超出模型限制。
func (c *ContextGuard) Compact(systemPrompt string, history []session.ChatMessage, newQuery string) ([]session.ChatMessage, string) {
	sysTokens := c.EstimateTokens(systemPrompt)
	queryTokens := c.EstimateTokens(newQuery)

	availableTokens := c.MaxTokens - sysTokens - queryTokens - 500 // 预留 500 tokens 给模型回复
	if availableTokens < 0 {
		return nil, newQuery
	}

	var compactedHistory []session.ChatMessage
	currentTokens := 0

	for i := len(history) - 1; i >= 0; i-- {
		msg := history[i]
		msgTokens := msg.Tokens
		if msgTokens == 0 {
			msgTokens = c.EstimateTokens(msg.Content)
		}

		if currentTokens+msgTokens > availableTokens {
			break 
		}
		
		currentTokens += msgTokens
		compactedHistory = append([]session.ChatMessage{msg}, compactedHistory...)
	}

	return compactedHistory, newQuery
}

// SemanticCompact 接收系统提示词、历史消息和新查询。
// 如果超出 MaxTokens，它会尝试保留最近的消息，并对由于太旧而被丢弃的消息生成语义摘要（可选）。
func (c *ContextGuard) SemanticCompact(ctx context.Context, p Provider, systemPrompt string, history []session.ChatMessage, newQuery string) ([]session.ChatMessage, string) {
	compacted, query := c.Compact(systemPrompt, history, newQuery)
	
	if len(compacted) < len(history) && p != nil {
		dropped := history[:len(history)-len(compacted)]
		if len(dropped) > 0 {
			summary, err := c.summarize(ctx, p, dropped)
			if err == nil {
				summaryMsg := session.ChatMessage{
					Role:    "system",
					Content: fmt.Sprintf("[Previous Context Summary]: %s", summary),
				}
				compacted = append([]session.ChatMessage{summaryMsg}, compacted...)
			}
		}
	}

	return compacted, query
}

func (c *ContextGuard) summarize(ctx context.Context, p Provider, msgs []session.ChatMessage) (string, error) {
	var sb strings.Builder
	sb.WriteString("Summarize the following conversation history briefly while preserving key facts and decisions:\n\n")
	for _, m := range msgs {
		sb.WriteString(fmt.Sprintf("[%s]: %s\n", m.Role, m.Content))
	}

	req := InferenceRequest{
		Messages: []ChatMessage{
			{Role: "user", Content: sb.String()},
		},
	}
	return p.Predict(ctx, req)
}

// PromptBuilder 用于组装最终发送给 LLM 的字符串
type PromptBuilder struct {
	Guard *ContextGuard
}

func NewPromptBuilder(maxTokens int) *PromptBuilder {
	return &PromptBuilder{Guard: NewContextGuard(maxTokens)}
}

// BuildText 组装标准的纯文本格式
func (pb *PromptBuilder) BuildText(systemPrompt string, history []session.ChatMessage, newQuery string) string {
	compacted, finalQuery := pb.Guard.Compact(systemPrompt, history, newQuery)

	var sb strings.Builder
	sb.WriteString(systemPrompt)
	sb.WriteString("\n\n")

	if len(compacted) > 0 {
		sb.WriteString("--- Chat History ---\n")
		for _, msg := range compacted {
			sb.WriteString(fmt.Sprintf("[%s]: %s\n", msg.Role, msg.Content))
		}
		sb.WriteString("--------------------\n\n")
	}

	sb.WriteString(fmt.Sprintf("[user]: %s\n", finalQuery))
	return sb.String()
}

// BuildMessages 组装 OpenAI 兼容的消息列表格式
func (pb *PromptBuilder) BuildMessages(systemPrompt string, history []session.ChatMessage, newQuery string) []ChatMessage {
	compacted, finalQuery := pb.Guard.Compact(systemPrompt, history, newQuery)
	
	messages := []ChatMessage{
		{Role: "system", Content: systemPrompt},
	}
	
	for _, h := range compacted {
		messages = append(messages, ChatMessage{
			Role:    h.Role,
			Content: h.Content,
		})
	}
	
	messages = append(messages, ChatMessage{
		Role:    "user",
		Content: finalQuery,
	})
	
	return messages
}
