package dingtalk

import (
	"agentic-core/internal/gateway"
	"context"
	"net/http"
)

const robotAdapterName = "dingtalk_robot"

type RobotAdapter struct {
	client robotSender
}

type robotSender interface {
	Send(ctx context.Context, msg gateway.ChannelResponse) error
}

func NewRobotAdapter(cfg RobotConfig, httpClient *http.Client) *RobotAdapter {
	return &RobotAdapter{client: newRobotClient(cfg, httpClient)}
}

func (a *RobotAdapter) Name() string {
	return robotAdapterName
}

func (a *RobotAdapter) SendMessage(ctx context.Context, sessionID string, text string) error {
	return a.Send(ctx, gateway.ChannelResponse{
		SessionID:   sessionID,
		ChannelName: a.Name(),
		MessageType: gateway.MessageTypeText,
		Format:      gateway.FormatPlainText,
		Text:        text,
	})
}

func (a *RobotAdapter) Send(ctx context.Context, msg gateway.ChannelResponse) error {
	if msg.ChannelName == "" {
		msg.ChannelName = a.Name()
	}
	return a.client.Send(ctx, msg)
}
