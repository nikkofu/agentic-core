package gateway

import (
	"agentic-core/internal/bus"
	"agentic-core/internal/llm"
	"context"
	"encoding/json"
	"testing"
	"time"
)

type mockSink struct {
	chunks []llm.StreamChunk
	closed bool
}

func (m *mockSink) WriteChunk(chunk llm.StreamChunk) error {
	m.chunks = append(m.chunks, chunk)
	return nil
}

func (m *mockSink) Close() error {
	m.closed = true
	return nil
}

type failingSink struct {
	writeCalls int
	closed     bool
}

func (f *failingSink) WriteChunk(chunk llm.StreamChunk) error {
	f.writeCalls++
	return context.Canceled
}

func (f *failingSink) Close() error {
	f.closed = true
	return nil
}

func TestSenderRouting(t *testing.T) {
	transport := bus.NewFakeTransport()
	sender := NewSender(transport)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := sender.Start(ctx); err != nil {
		t.Fatalf("failed to start sender: %v", err)
	}

	sink := &mockSink{}
	sender.RegisterSink("task-123", sink)

	// 模拟发送 Chunk 到 Redis
	chunk := llm.StreamChunk{
		TaskID:   "task-123",
		Event:    "delta",
		Delta:    "hello",
		Sequence: 1,
	}
	payload, _ := json.Marshal(chunk)
	// 注意：FakePubSub.Publish 会将 payload 放入 Message
	_ = transport.Publish(ctx, "chunks.task-123", bus.Message{
		MessageID:  "chunk-1",
		SenderID:   "runtime",
		ReceiverID: "sender",
		Payload:    payload,
		Timestamp:  time.Now().UnixMilli(),
	})

	// 等待分发
	time.Sleep(200 * time.Millisecond)

	if len(sink.chunks) != 1 {
		t.Errorf("expected 1 chunk, got %d", len(sink.chunks))
	} else if sink.chunks[0].Delta != "hello" {
		t.Errorf("chunk content mismatch: %v", sink.chunks[0].Delta)
	}
}

func TestSenderClosesAndUnregistersSinkOnTerminalChunk(t *testing.T) {
	transport := bus.NewFakeTransport()
	sender := NewSender(transport)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := sender.Start(ctx); err != nil {
		t.Fatalf("failed to start sender: %v", err)
	}

	sink := &mockSink{}
	sender.RegisterSink("task-123", sink)

	chunk := llm.StreamChunk{
		TaskID:   "task-123",
		Event:    "final",
		Done:     true,
		Sequence: 1,
	}
	payload, _ := json.Marshal(chunk)
	if err := transport.Publish(ctx, "chunks.task-123", bus.Message{
		MessageID:  "chunk-1",
		SenderID:   "runtime",
		ReceiverID: "sender",
		Payload:    payload,
		Timestamp:  time.Now().UnixMilli(),
	}); err != nil {
		t.Fatalf("publish failed: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	if len(sink.chunks) != 1 {
		t.Fatalf("expected terminal chunk delivered once, got %d", len(sink.chunks))
	}
	if !sink.closed {
		t.Fatal("expected sink to be closed after terminal chunk")
	}

	sender.mu.RLock()
	_, ok := sender.sinks["task-123"]
	sender.mu.RUnlock()
	if ok {
		t.Fatal("expected sink unregistered after terminal chunk")
	}
}

func TestSenderDisconnectDoesNotEmitDoneFrame(t *testing.T) {
	transport := bus.NewFakeTransport()
	sender := NewSender(transport)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := sender.Start(ctx); err != nil {
		t.Fatalf("failed to start sender: %v", err)
	}

	sink := &failingSink{}
	sender.RegisterSink("task-123", sink)

	payload, _ := json.Marshal(llm.StreamChunk{
		TaskID:   "task-123",
		Event:    "delta",
		Delta:    "hello",
		Sequence: 1,
	})
	if err := transport.Publish(ctx, "chunks.task-123", bus.Message{
		MessageID:  "chunk-1",
		SenderID:   "runtime",
		ReceiverID: "sender",
		Payload:    payload,
		Timestamp:  time.Now().UnixMilli(),
	}); err != nil {
		t.Fatalf("publish failed: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	if sink.writeCalls != 1 {
		t.Fatalf("expected exactly one failed write attempt, got %d", sink.writeCalls)
	}
	if sink.closed {
		t.Fatal("expected disconnect path not to emit done/close frame")
	}

	sender.mu.RLock()
	_, ok := sender.sinks["task-123"]
	sender.mu.RUnlock()
	if ok {
		t.Fatal("expected sink unregistered after disconnect")
	}
}

func TestSenderPublishesAbortedStreamAuditOnDisconnect(t *testing.T) {
	transport := bus.NewFakeTransport()
	sender := NewSender(transport)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	auditEvents, err := transport.Subscribe(ctx, "audit.task-123")
	if err != nil {
		t.Fatalf("subscribe audit failed: %v", err)
	}

	if err := sender.Start(ctx); err != nil {
		t.Fatalf("failed to start sender: %v", err)
	}

	sink := &failingSink{}
	sender.RegisterSink("task-123", sink)

	payload, _ := json.Marshal(llm.StreamChunk{
		TraceID:   "trace-1",
		SessionID: "session-1",
		TaskID:    "task-123",
		Event:     "delta",
		Delta:     "hello",
		Sequence:  1,
	})
	if err := transport.Publish(ctx, "chunks.task-123", bus.Message{
		MessageID:  "chunk-1",
		SenderID:   "runtime",
		ReceiverID: "sender",
		Payload:    payload,
		Timestamp:  time.Now().UnixMilli(),
	}); err != nil {
		t.Fatalf("publish failed: %v", err)
	}

	select {
	case msg := <-auditEvents:
		var event llm.AuditEvent
		if err := json.Unmarshal(msg.Payload, &event); err != nil {
			t.Fatalf("unmarshal audit event failed: %v", err)
		}
		if event.Event != "aborted_stream" {
			t.Fatalf("expected aborted_stream event, got %s", event.Event)
		}
		if event.Status != "aborted" {
			t.Fatalf("expected aborted status, got %s", event.Status)
		}
		if event.Error == "" {
			t.Fatal("expected sink error preserved in audit event")
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for aborted_stream audit event")
	}
}

func TestSenderPublishesDoneStatusAuditWhenFinalChunkDisconnects(t *testing.T) {
	transport := bus.NewFakeTransport()
	sender := NewSender(transport)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	auditEvents, err := transport.Subscribe(ctx, "audit.task-123")
	if err != nil {
		t.Fatalf("subscribe audit failed: %v", err)
	}

	if err := sender.Start(ctx); err != nil {
		t.Fatalf("failed to start sender: %v", err)
	}

	sink := &failingSink{}
	sender.RegisterSink("task-123", sink)

	payload, _ := json.Marshal(llm.StreamChunk{
		TraceID:   "trace-1",
		SessionID: "session-1",
		TaskID:    "task-123",
		Event:     "final",
		Done:      true,
		Sequence:  2,
	})
	if err := transport.Publish(ctx, "chunks.task-123", bus.Message{
		MessageID:  "chunk-2",
		SenderID:   "runtime",
		ReceiverID: "sender",
		Payload:    payload,
		Timestamp:  time.Now().UnixMilli(),
	}); err != nil {
		t.Fatalf("publish failed: %v", err)
	}

	select {
	case msg := <-auditEvents:
		var event llm.AuditEvent
		if err := json.Unmarshal(msg.Payload, &event); err != nil {
			t.Fatalf("unmarshal audit event failed: %v", err)
		}
		if event.Event != "aborted_stream" {
			t.Fatalf("expected aborted_stream event, got %s", event.Event)
		}
		if event.Status != "done" {
			t.Fatalf("expected done status for terminal aborted stream, got %s", event.Status)
		}

		var data map[string]any
		if err := json.Unmarshal(event.Data, &data); err != nil {
			t.Fatalf("unmarshal aborted_stream data failed: %v", err)
		}
		if data["stream_event"] != "final" {
			t.Fatalf("expected stream_event final, got %+v", data["stream_event"])
		}
		if data["done"] != true {
			t.Fatalf("expected done=true in audit data, got %+v", data["done"])
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for terminal aborted_stream audit event")
	}

	if sink.closed {
		t.Fatal("expected disconnect on final chunk not to emit done/close frame")
	}
}
