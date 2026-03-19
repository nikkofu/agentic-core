package feishu

import (
	"agentic-core/internal/gateway"
	"context"
	"net/http"
)

const botAdapterName = "feishu_bot"

type BotAdapter struct {
	client *BotClient
}

func NewBotAdapter(cfg BotConfig, httpClient *http.Client) *BotAdapter {
	return &BotAdapter{
		client: newBotClient(cfg, httpClient),
	}
}

func (a *BotAdapter) Name() string {
	return botAdapterName
}

func (a *BotAdapter) SendMessage(ctx context.Context, sessionID string, text string) error {
	return a.Send(ctx, gateway.ChannelResponse{
		SessionID:   sessionID,
		ChannelName: a.Name(),
		MessageType: gateway.MessageTypeText,
		Format:      gateway.FormatPlainText,
		Text:        text,
	})
}

func (a *BotAdapter) Send(ctx context.Context, msg gateway.ChannelResponse) error {
	if msg.ChannelName == "" {
		msg.ChannelName = a.Name()
	}
	return a.client.Send(ctx, msg)
}
