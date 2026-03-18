package gateway

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"agentic-core/internal/bus"
)

type capturingRichAdapter struct {
	name     string
	messages []ChannelResponse
}

func (a *capturingRichAdapter) Name() string {
	return a.name
}

func (a *capturingRichAdapter) SendMessage(ctx context.Context, sessionID string, text string) error {
	a.messages = append(a.messages, ChannelResponse{
		SessionID: sessionID,
		Text:      text,
	})
	return nil
}

func (a *capturingRichAdapter) Send(ctx context.Context, msg ChannelResponse) error {
	a.messages = append(a.messages, msg)
	return nil
}

func TestHandleIncomingTracksTaskRouteAndEnqueuesUnifiedPayload(t *testing.T) {
	transport := bus.NewFakeTransport()
	router := NewSessionRouter(transport)

	req := ChannelRequest{
		SessionID:   "user-1",
		ChannelName: "wecom",
		SenderID:    "zhangsan",
		SenderName:  "张三",
		MessageID:   "msg-1",
		MessageType: MessageTypeImage,
		Text:        "请识别这张图",
		Media: []MediaItem{
			{
				Kind:    MediaKindImage,
				MediaID: "MEDIA123",
			},
		},
		Metadata: map[string]any{
			"tenant":       "demo",
			"wecom_touser": "zhangsan",
		},
	}

	if err := router.HandleIncoming(context.Background(), req); err != nil {
		t.Fatalf("HandleIncoming failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	msgChan, err := transport.Dequeue(ctx, "tasks")
	if err != nil {
		t.Fatalf("Dequeue failed: %v", err)
	}

	var enqueued bus.Message
	select {
	case enqueued = <-msgChan:
	case <-ctx.Done():
		t.Fatal("timed out waiting for enqueued task")
	}

	var payload map[string]any
	if err := json.Unmarshal(enqueued.Payload, &payload); err != nil {
		t.Fatalf("unmarshal payload failed: %v", err)
	}

	if got := payload["channel"]; got != "wecom" {
		t.Fatalf("expected channel wecom, got %v", got)
	}
	if got := payload["message_type"]; got != string(MessageTypeImage) {
		t.Fatalf("expected message_type=image, got %v", got)
	}
	if got := payload["sender_id"]; got != "zhangsan" {
		t.Fatalf("expected sender_id zhangsan, got %v", got)
	}
	media, ok := payload["media"].([]any)
	if !ok || len(media) != 1 {
		t.Fatalf("expected one media item, got %#v", payload["media"])
	}

	binding, ok := router.routes[enqueued.MessageID]
	if !ok {
		t.Fatalf("expected route binding for task %s", enqueued.MessageID)
	}
	if binding.SessionID != "user-1" {
		t.Fatalf("expected stored session user-1, got %s", binding.SessionID)
	}
	if binding.ChannelName != "wecom" {
		t.Fatalf("expected stored channel wecom, got %s", binding.ChannelName)
	}
}

func TestStartStreamListenerRoutesRichMessageToRegisteredAdapter(t *testing.T) {
	transport := bus.NewFakeTransport()
	router := NewSessionRouter(transport)
	adapter := &capturingRichAdapter{name: "wecom"}
	router.RegisterAdapter(adapter)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := router.StartStreamListener(ctx); err != nil {
		t.Fatalf("StartStreamListener failed: %v", err)
	}

	req := ChannelRequest{
		SessionID:   "user-9",
		ChannelName: "wecom",
		Text:        "hello",
		Metadata: map[string]any{
			"wecom_touser": "lisi",
		},
	}
	if err := router.HandleIncoming(ctx, req); err != nil {
		t.Fatalf("HandleIncoming failed: %v", err)
	}

	taskChan, err := transport.Dequeue(ctx, "tasks")
	if err != nil {
		t.Fatalf("Dequeue tasks failed: %v", err)
	}

	var task bus.Message
	select {
	case task = <-taskChan:
	case <-ctx.Done():
		t.Fatal("timed out waiting for task")
	}

	outbound := ChannelResponse{
		MessageType: MessageTypeMarkdown,
		Format:      FormatMarkdown,
		Text:        "**完成**",
	}
	output, err := json.Marshal(map[string]any{
		"message": outbound,
	})
	if err != nil {
		t.Fatalf("marshal output failed: %v", err)
	}

	resultPayload, err := json.Marshal(bus.TaskResult{
		TaskID:    task.MessageID,
		AgentName: "orchestrator",
		Status:    "success",
		Output:    output,
		Timestamp: time.Now().UnixMilli(),
	})
	if err != nil {
		t.Fatalf("marshal task result failed: %v", err)
	}

	if err := transport.Enqueue(ctx, "task_results", bus.Message{
		MessageID:  task.MessageID + ".result",
		SenderID:   "orchestrator",
		ReceiverID: "gateway",
		Payload:    resultPayload,
		Timestamp:  time.Now().UnixMilli(),
	}); err != nil {
		t.Fatalf("enqueue task result failed: %v", err)
	}

	deadline := time.Now().Add(1500 * time.Millisecond)
	for len(adapter.messages) == 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if len(adapter.messages) != 1 {
		t.Fatalf("expected one outbound message, got %d", len(adapter.messages))
	}

	got := adapter.messages[0]
	if got.SessionID != "user-9" {
		t.Fatalf("expected routed session user-9, got %s", got.SessionID)
	}
	if got.ChannelName != "wecom" {
		t.Fatalf("expected channel wecom, got %s", got.ChannelName)
	}
	if got.Format != FormatMarkdown {
		t.Fatalf("expected markdown format, got %s", got.Format)
	}
	if got.Text != "**完成**" {
		t.Fatalf("expected markdown body preserved, got %s", got.Text)
	}
	if got.Metadata["wecom_touser"] != "lisi" {
		t.Fatalf("expected original route metadata merged, got %#v", got.Metadata)
	}
	if _, ok := router.routes[task.MessageID]; ok {
		t.Fatalf("expected route binding cleaned after reply for task %s", task.MessageID)
	}
}

func TestStartStreamListenerDirectSendUsesExplicitChannelMessage(t *testing.T) {
	transport := bus.NewFakeTransport()
	router := NewSessionRouter(transport)
	adapter := &capturingRichAdapter{name: "feishu_bot"}
	router.RegisterAdapter(adapter)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := router.StartStreamListener(ctx); err != nil {
		t.Fatalf("StartStreamListener failed: %v", err)
	}

	output, err := json.Marshal(map[string]any{
		"message": ChannelResponse{
			ChannelName: "feishu_bot",
			MessageType: MessageTypeText,
			Text:        "notify",
			Metadata: map[string]any{
				"tenant": "demo",
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal output failed: %v", err)
	}

	resultPayload, err := json.Marshal(bus.TaskResult{
		TaskID:    "task-direct-send",
		AgentName: "orchestrator",
		Status:    "success",
		Output:    output,
		Timestamp: time.Now().UnixMilli(),
	})
	if err != nil {
		t.Fatalf("marshal task result failed: %v", err)
	}

	if err := transport.Enqueue(ctx, "task_results", bus.Message{
		MessageID:  "task-direct-send.result",
		SenderID:   "orchestrator",
		ReceiverID: "gateway",
		Payload:    resultPayload,
		Timestamp:  time.Now().UnixMilli(),
	}); err != nil {
		t.Fatalf("enqueue task result failed: %v", err)
	}

	deadline := time.Now().Add(1500 * time.Millisecond)
	for len(adapter.messages) == 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if len(adapter.messages) != 1 {
		t.Fatalf("expected one outbound message, got %d", len(adapter.messages))
	}

	got := adapter.messages[0]
	if got.ChannelName != "feishu_bot" {
		t.Fatalf("expected channel feishu_bot, got %s", got.ChannelName)
	}
	if got.Text != "notify" {
		t.Fatalf("expected text notify, got %s", got.Text)
	}
	if got.Metadata["tenant"] != "demo" {
		t.Fatalf("expected metadata merged for direct send, got %#v", got.Metadata)
	}
}

func TestStartStreamListenerDirectSendOverridesRouteBinding(t *testing.T) {
	transport := bus.NewFakeTransport()
	router := NewSessionRouter(transport)
	wecomAdapter := &capturingRichAdapter{name: "wecom"}
	botAdapter := &capturingRichAdapter{name: "feishu_bot"}
	router.RegisterAdapter(wecomAdapter)
	router.RegisterAdapter(botAdapter)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := router.StartStreamListener(ctx); err != nil {
		t.Fatalf("StartStreamListener failed: %v", err)
	}

	taskID := "task-explicit-channel"
	router.rememberRoute(taskID, ChannelRequest{
		SessionID:   "legacy-session",
		ChannelName: "wecom",
		Metadata: map[string]any{
			"wecom_touser": "lisi",
			"route_only":   "binding",
		},
	})

	output, err := json.Marshal(map[string]any{
		"message": ChannelResponse{
			ChannelName: "feishu_bot",
			MessageType: MessageTypeText,
			Text:        "explicit channel wins",
			Metadata: map[string]any{
				"send_mode": "direct",
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal output failed: %v", err)
	}

	resultPayload, err := json.Marshal(bus.TaskResult{
		TaskID:    taskID,
		AgentName: "orchestrator",
		Status:    "success",
		Output:    output,
		Timestamp: time.Now().UnixMilli(),
	})
	if err != nil {
		t.Fatalf("marshal task result failed: %v", err)
	}

	if err := transport.Enqueue(ctx, "task_results", bus.Message{
		MessageID:  taskID + ".result",
		SenderID:   "orchestrator",
		ReceiverID: "gateway",
		Payload:    resultPayload,
		Timestamp:  time.Now().UnixMilli(),
	}); err != nil {
		t.Fatalf("enqueue task result failed: %v", err)
	}

	deadline := time.Now().Add(1500 * time.Millisecond)
	for len(botAdapter.messages) == 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}

	if len(botAdapter.messages) != 1 {
		t.Fatalf("expected one bot outbound message, got %d", len(botAdapter.messages))
	}
	if len(wecomAdapter.messages) != 0 {
		t.Fatalf("expected no wecom outbound message, got %d", len(wecomAdapter.messages))
	}

	got := botAdapter.messages[0]
	if got.ChannelName != "feishu_bot" {
		t.Fatalf("expected channel feishu_bot, got %s", got.ChannelName)
	}
	if got.Text != "explicit channel wins" {
		t.Fatalf("expected explicit text, got %s", got.Text)
	}
	if got.Metadata["send_mode"] != "direct" {
		t.Fatalf("expected explicit metadata preserved, got %#v", got.Metadata)
	}
	if got.Metadata["route_only"] != "binding" {
		t.Fatalf("expected route metadata merged, got %#v", got.Metadata)
	}
}
