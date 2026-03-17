package bus

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
)

type Message struct {
	MessageID    string          `json:"message_id"`
	ParentTaskID string          `json:"parent_task_id,omitempty"` // 用于关联父任务
	SenderID     string          `json:"sender_id"`
	ReceiverID   string          `json:"receiver_id"`
	TargetAgent  string          `json:"target_agent,omitempty"`   // 目标 Agent 名称 (例如 "researcher")
	Payload      json.RawMessage `json:"payload"`
	Timestamp    int64           `json:"timestamp"`
}

type TaskResult struct {
	TaskID       string          `json:"task_id"`
	ParentTaskID string          `json:"parent_task_id,omitempty"`
	AgentName    string          `json:"agent_name"` // 执行任务的 Agent 名称
	Status       string          `json:"status"`     // "success", "failed", "running"
	Output       json.RawMessage `json:"output"`
	Error        string          `json:"error,omitempty"`
	Timestamp    int64           `json:"timestamp"`
}

type ApprovalRequest struct {
	TaskID    string `json:"task_id"`
	Operation string `json:"operation"` // 例如 "delete_file", "send_email"
	Details   string `json:"details"`
}

type ApprovalResponse struct {
	TaskID   string `json:"task_id"`
	Approved bool   `json:"approved"`
	Reason   string `json:"reason,omitempty"`
}

func ParseMessage(data []byte) (Message, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()

	var msg Message
	if err := dec.Decode(&msg); err != nil {
		return Message{}, err
	}
	if err := msg.Validate(); err != nil {
		return Message{}, err
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return Message{}, errors.New("trailing json data")
	}
	return msg, nil
}

func (m Message) Validate() error {
	if m.MessageID == "" {
		return errors.New("message_id is required")
	}
	if m.SenderID == "" {
		return errors.New("sender_id is required")
	}
	if m.ReceiverID == "" {
		return errors.New("receiver_id is required")
	}
	if m.Timestamp <= 0 {
		return errors.New("timestamp must be positive")
	}
	if len(m.Payload) == 0 {
		return errors.New("payload is required")
	}
	if !json.Valid(m.Payload) {
		return errors.New("payload must be valid json")
	}
	return nil
}
