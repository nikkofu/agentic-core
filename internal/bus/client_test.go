package bus

import (
	"context"
	"errors"
	"testing"
)

func TestFakePubSubPublishAndConsume(t *testing.T) {
	ps := NewFakePubSub()
	ctx := context.Background()

	msg := Message{
		MessageID:  "m1",
		SenderID:   "orch",
		ReceiverID: "sub-1",
		Payload:    []byte(`{"task":"run"}`),
		Timestamp:  1735689600000,
	}
	if err := ps.Publish(ctx, "tasks", msg); err != nil {
		t.Fatalf("publish failed: %v", err)
	}

	got, err := ps.Consume(ctx, "tasks")
	if err != nil {
		t.Fatalf("consume failed: %v", err)
	}
	if got.MessageID != "m1" {
		t.Fatalf("unexpected message id: %s", got.MessageID)
	}
}

func TestFakePubSubConsumeEmptyReturnsErrNoMessage(t *testing.T) {
	ps := NewFakePubSub()
	_, err := ps.Consume(context.Background(), "tasks")
	if !errors.Is(err, ErrNoMessage) {
		t.Fatalf("expected ErrNoMessage, got %v", err)
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
