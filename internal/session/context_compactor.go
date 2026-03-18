package session

import (
	"fmt"
	"strings"
)

// CompactorPolicy 定义压缩策略
type CompactorPolicy struct {
	MaxTokens  int
	KeepRecent int // 保留最近的 N 条消息不参与摘要
}

// CompactHistory 压缩历史消息
// 如果消息总数超过 KeepRecent，则将较早的消息提取出来用于摘要
func CompactHistory(messages []ChatMessage, policy CompactorPolicy) (toSummarize []ChatMessage, kept []ChatMessage) {
	if len(messages) <= policy.KeepRecent {
		return nil, messages
	}

	splitIdx := len(messages) - policy.KeepRecent
	toSummarize = messages[:splitIdx]
	kept = messages[splitIdx:]

	return toSummarize, kept
}

// FormatForSummary 将消息格式化为待摘要的文本
func FormatForSummary(messages []ChatMessage) string {
	var sb strings.Builder
	for _, m := range messages {
		sb.WriteString(fmt.Sprintf("[%s]: %s\n", m.Role, m.Content))
	}
	return sb.String()
}
