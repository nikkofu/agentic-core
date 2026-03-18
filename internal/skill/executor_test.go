package skill

import (
	"agentic-core/internal/llm"
	"context"
	"encoding/json"
	"testing"
)

func TestExecutorRunsBuiltinTool(t *testing.T) {
	registry := NewRegistry()
	registry.Register(&CurrentTimeSkill{})
	executor := NewExecutor(registry)

	call := llm.ToolCall{
		ID:        "call-1",
		Name:      "get_current_time",
		Arguments: json.RawMessage(`{}`),
	}

	result, err := executor.Execute(context.Background(), call)
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	if !result.Success {
		t.Errorf("expected success, got error: %s", result.Error)
	}

	if result.ToolCallID != "call-1" {
		t.Errorf("expected tool call id call-1, got %s", result.ToolCallID)
	}

	var output map[string]string
	if err := json.Unmarshal([]byte(result.Output), &output); err != nil {
		t.Fatalf("failed to unmarshal output: %v", err)
	}

	if _, ok := output["time"]; !ok {
		t.Error("expected 'time' field in output")
	}
}

func TestExecutorReturnsErrorForUnknownTool(t *testing.T) {
	registry := NewRegistry()
	executor := NewExecutor(registry)

	call := llm.ToolCall{
		ID:   "call-2",
		Name: "unknown_tool",
	}

	result, err := executor.Execute(context.Background(), call)
	if err == nil {
		t.Error("expected error for unknown tool")
	}

	if result.Success {
		t.Error("expected success to be false")
	}

	if result.Error == "" {
		t.Error("expected error message in result")
	}
}
