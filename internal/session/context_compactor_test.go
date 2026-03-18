package session

import (
	"testing"
)

func TestCompactHistory(t *testing.T) {
	messages := []ChatMessage{
		{Role: "user", Content: "1"},
		{Role: "assistant", Content: "2"},
		{Role: "user", Content: "3"},
		{Role: "assistant", Content: "4"},
		{Role: "user", Content: "5"},
	}

	policy := CompactorPolicy{KeepRecent: 2}
	toSum, kept := CompactHistory(messages, policy)

	if len(kept) != 2 {
		t.Errorf("expected 2 kept messages, got %d", len(kept))
	}
	if len(toSum) != 3 {
		t.Errorf("expected 3 messages to summarize, got %d", len(toSum))
	}

	if kept[0].Content != "4" || kept[1].Content != "5" {
		t.Errorf("kept messages mismatch")
	}
}
