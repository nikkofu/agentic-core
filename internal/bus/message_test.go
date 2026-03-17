package bus

import (
	"encoding/json"
	"testing"
)

func TestMessageValidateRequiresIDsAndTimestamp(t *testing.T) {
	msg := Message{
		Payload:   json.RawMessage(`{"k":"v"}`),
		Timestamp: 0,
	}

	if err := msg.Validate(); err == nil {
		t.Fatal("expected validation error for missing IDs and invalid timestamp")
	}
}

func TestMessageValidateRejectsInvalidPayloadJSON(t *testing.T) {
	msg := Message{
		MessageID:  "m1",
		SenderID:   "orchestrator",
		ReceiverID: "subagent-1",
		Payload:    json.RawMessage(`{"task":`),
		Timestamp:  1735689600000,
	}

	if err := msg.Validate(); err == nil {
		t.Fatal("expected validation error for invalid payload json")
	}
}

func TestMessageValidateAcceptsValidMessage(t *testing.T) {
	msg := Message{
		MessageID:  "m1",
		SenderID:   "orchestrator",
		ReceiverID: "subagent-1",
		Payload:    json.RawMessage(`{"task":"run"}`),
		Timestamp:  1735689600000,
	}

	if err := msg.Validate(); err != nil {
		t.Fatalf("expected valid message, got error: %v", err)
	}
}

func TestParseMessageRejectsUnknownFields(t *testing.T) {
	_, err := ParseMessage([]byte(`{"message_id":"m1","sender_id":"orchestrator","receiver_id":"sub-1","payload":{},"timestamp":1735689600000,"extra":"x"}`))
	if err == nil {
		t.Fatal("expected parse error for unknown field")
	}
}

func TestParseMessageRejectsTrailingJSON(t *testing.T) {
	_, err := ParseMessage([]byte(`{"message_id":"m1","sender_id":"orchestrator","receiver_id":"sub-1","payload":{},"timestamp":1735689600000} {"message_id":"m2"}`))
	if err == nil {
		t.Fatal("expected parse error for trailing json")
	}
}

func TestParseMessageAcceptsValidEnvelope(t *testing.T) {
	msg, err := ParseMessage([]byte(`{"message_id":"m1","sender_id":"orchestrator","receiver_id":"sub-1","payload":{"task":"run"},"timestamp":1735689600000}`))
	if err != nil {
		t.Fatalf("expected parse success, got error: %v", err)
	}
	if msg.MessageID != "m1" {
		t.Fatalf("unexpected message id: %s", msg.MessageID)
	}
}
