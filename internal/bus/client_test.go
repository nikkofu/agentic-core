package bus

import (
	"context"
	"testing"
	"time"
)

func TestFakePubSubPublishAndConsume(t *testing.T) {
	transport := NewFakeTransport()
	ctx := context.Background()

	msg := Message{
		MessageID:  "m1",
		SenderID:   "orch",
		ReceiverID: "sub-1",
		Payload:    []byte(`{"task":"run"}`),
		Timestamp:  1735689600000,
	}
	if err := transport.Enqueue(ctx, "tasks", msg); err != nil {
		t.Fatalf("enqueue failed: %v", err)
	}

	ch, err := transport.Dequeue(ctx, "tasks")
	if err != nil {
		t.Fatalf("dequeue failed: %v", err)
	}

	select {
	case got := <-ch:
		if got.MessageID != "m1" {
			t.Fatalf("unexpected message id: %s", got.MessageID)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("expected message to be available in channel")
	}
}

func TestFakeTransportPublishAndSubscribe(t *testing.T) {
	transport := NewFakeTransport()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch, err := transport.Subscribe(ctx, "chunks.*")
	if err != nil {
		t.Fatalf("subscribe failed: %v", err)
	}

	msg := Message{
		MessageID:  "m2",
		SenderID:   "runtime",
		ReceiverID: "sender",
		Payload:    []byte(`{"event":"delta"}`),
		Timestamp:  1735689600001,
	}
	if err := transport.Publish(ctx, "chunks.task-1", msg); err != nil {
		t.Fatalf("publish failed: %v", err)
	}

	select {
	case got := <-ch:
		if got.MessageID != "m2" {
			t.Fatalf("unexpected message id: %s", got.MessageID)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("expected event to be available to subscriber")
	}
}

func TestFakeTransportWildcardMatchesOnlyDottedNamespace(t *testing.T) {
	transport := NewFakeTransport()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch, err := transport.Subscribe(ctx, "chunks.*")
	if err != nil {
		t.Fatalf("subscribe failed: %v", err)
	}

	if err := transport.Publish(ctx, "chunksx", Message{
		MessageID:  "m-wrong",
		SenderID:   "runtime",
		ReceiverID: "sender",
		Payload:    []byte(`{"event":"delta"}`),
		Timestamp:  1735689600002,
	}); err != nil {
		t.Fatalf("publish wrong topic failed: %v", err)
	}

	select {
	case got := <-ch:
		t.Fatalf("expected wildcard namespace miss, got unexpected message %s", got.MessageID)
	case <-time.After(50 * time.Millisecond):
	}

	if err := transport.Publish(ctx, "chunks.task-1", Message{
		MessageID:  "m-right",
		SenderID:   "runtime",
		ReceiverID: "sender",
		Payload:    []byte(`{"event":"delta"}`),
		Timestamp:  1735689600003,
	}); err != nil {
		t.Fatalf("publish right topic failed: %v", err)
	}

	select {
	case got := <-ch:
		if got.MessageID != "m-right" {
			t.Fatalf("expected message m-right, got %s", got.MessageID)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("expected dotted namespace event to be available to subscriber")
	}
}

func TestFakeTransportDequeueClosesOnCancel(t *testing.T) {
	transport := NewFakeTransport()
	ctx, cancel := context.WithCancel(context.Background())
	ch, err := transport.Dequeue(ctx, "tasks")
	if err != nil {
		t.Fatalf("dequeue failed: %v", err)
	}

	cancel()

	_, ok := <-ch
	if ok {
		t.Fatal("expected dequeue channel to close after cancel")
	}
}

func TestFakeHeartbeatPublishAndLast(t *testing.T) {
	hb := NewFakeHeartbeatBus()
	ctx := context.Background()

	if err := hb.PublishHeartbeat(ctx, "sub-1", "running"); err != nil {
		t.Fatalf("publish heartbeat failed: %v", err)
	}

	status, ok := hb.LastStatus("sub-1")
	if !ok {
		t.Fatal("expected heartbeat status to exist")
	}
	if status != "running" {
		t.Fatalf("expected running status, got %s", status)
	}
}
