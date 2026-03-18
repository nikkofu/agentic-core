package llm

import (
	"encoding/json"
)

// ChatMessage 与 OpenAI chat/completions 协议对齐
type ChatMessage struct {
	Role       string          `json:"role"`
	Content    string          `json:"content"`
	Name       string          `json:"name,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
	ToolCalls  json.RawMessage `json:"tool_calls,omitempty"`
}

// InferenceRequest 内部推理请求
type InferenceRequest struct {
	TraceID          string            `json:"trace_id"`
	SessionID        string            `json:"session_id"`
	TaskID           string            `json:"task_id"`
	AgentType        string            `json:"agent_type"`
	Messages         []ChatMessage     `json:"messages"`
	ModelAlias       string            `json:"model_alias"`
	MaxTurns         int               `json:"max_turns"`
	Stream           bool              `json:"stream"`
	Temperature      *float32          `json:"temperature,omitempty"`
	Metadata         map[string]string `json:"metadata,omitempty"`
	OnApprovalReject string            `json:"on_approval_reject"` // continue | fail
}

// ActionEnvelope 定义了模型输出的确定性包装
type ActionEnvelope struct {
	Think     string          `json:"think"`               // 思考过程
	CallSkill *ToolCall       `json:"call_skill,omitempty"` // 调用 Skill
	Final     string          `json:"final,omitempty"`      // 最终回复
}

// ToolCall 结构定义
type ToolCall struct {
	ID               string          `json:"id"`
	Name             string          `json:"name"`
	Arguments        json.RawMessage `json:"arguments"`
	IsWriteOperation bool            `json:"is_write_operation"`
}

type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // JSON string
}

// ToolResult 结构定义
type ToolResult struct {
	ToolCallID string `json:"tool_call_id"`
	Name       string `json:"name"`
	Success    bool   `json:"success"`
	Output     string `json:"output"`
	Error      string `json:"error"`
	DurationMs int64  `json:"duration_ms"`
}

// StreamChunk 统一流式事件模型
type StreamChunk struct {
	TraceID     string          `json:"trace_id"`
	SessionID   string          `json:"session_id"`
	TaskID      string          `json:"task_id"`
	Sequence    int64           `json:"sequence"`
	Event       string          `json:"event"` // delta | tool_call | tool_result | final | error | heartbeat | done
	Delta       string          `json:"delta,omitempty"`
	Role        string          `json:"role,omitempty"`
	ToolName    string          `json:"tool_name,omitempty"`
	Done        bool            `json:"done"`
	Error       string          `json:"error,omitempty"`
	Data        json.RawMessage `json:"data,omitempty"`
	TimestampMs int64           `json:"timestamp_ms"`
}

// FinalResult 推理最终结果
type FinalResult struct {
	TraceID     string `json:"trace_id"`
	TaskID      string `json:"task_id"`
	Content     string `json:"content"`
	Status      string `json:"status"` // success | error | timeout | cancelled
	TokensUsed  int    `json:"tokens_used"`
	TimestampMs int64  `json:"timestamp_ms"`
}

// ApprovalRequest 审批请求
type ApprovalRequest struct {
	TraceID       string          `json:"trace_id"`
	TaskID        string          `json:"task_id"`
	ToolCallID    string          `json:"tool_call_id"`
	ToolName      string          `json:"tool_name"`
	Arguments     json.RawMessage `json:"arguments"`
	RequestedAtMs int64           `json:"requested_at_ms"`
}

// ApprovalDecision 审批决策
type ApprovalDecision struct {
	TraceID     string `json:"trace_id"`
	TaskID      string `json:"task_id"`
	ToolCallID  string `json:"tool_call_id"`
	Approved    bool   `json:"approved"`
	Reviewer    string `json:"reviewer"`
	Reason      string `json:"reason"`
	DecidedAtMs int64  `json:"decided_at_ms"`
}

// ChatCompletionRequest 对应 OpenAI API 请求
type ChatCompletionRequest struct {
	Model       string          `json:"model"`
	Messages    []ChatMessage   `json:"messages"`
	Tools       json.RawMessage `json:"tools,omitempty"`
	ToolChoice  json.RawMessage `json:"tool_choice,omitempty"`
	Temperature *float32        `json:"temperature,omitempty"`
	Stream      bool            `json:"stream,omitempty"`
}
