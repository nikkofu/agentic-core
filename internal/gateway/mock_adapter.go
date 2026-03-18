package gateway

import (
	"agentic-core/internal/logging"
	"context"
)

// MockAdapter 是一个用于演示和测试的伪造适配器
type MockAdapter struct {
	channelName string
}

func NewMockAdapter(name string) *MockAdapter {
	return &MockAdapter{channelName: name}
}

func (m *MockAdapter) Name() string {
	return m.channelName
}

func (m *MockAdapter) SendMessage(ctx context.Context, sessionID string, text string) error {
	logging.Component("gateway.mock_adapter").Info("sending mock adapter message",
		"channel_name", m.channelName,
		"session_id", sessionID,
		"text", text,
	)
	return nil
}
