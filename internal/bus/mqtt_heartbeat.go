package bus

import (
	"context"
	"fmt"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

type MQTTHeartbeatBus struct {
	client mqtt.Client
}

func NewMQTTHeartbeatBus(client mqtt.Client) *MQTTHeartbeatBus {
	return &MQTTHeartbeatBus{client: client}
}

func (m *MQTTHeartbeatBus) PublishHeartbeat(ctx context.Context, agentID string, status string) error {
	topic := fmt.Sprintf("agents/%s/heartbeat", agentID)
	payload := status
	
	// MQTT 的 Token 机制不是 Context 原生支持的，我们需要适配
	token := m.client.Publish(topic, 1, false, payload)
	
	// 在 background 或 context 取消时等待结果
	done := make(chan struct{})
	go func() {
		token.Wait()
		close(done)
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
		return token.Error()
	}
}
