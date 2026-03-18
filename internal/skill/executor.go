package skill

import (
	"agentic-core/internal/llm"
	"context"
	"fmt"
	"time"
)

// Executor 负责统一执行各种来源的工具 (内置, WASM 等)
type Executor struct {
	registry *Registry
}

func NewExecutor(registry *Registry) *Executor {
	return &Executor{
		registry: registry,
	}
}

// Execute 执行一个工具调用
func (e *Executor) Execute(ctx context.Context, call llm.ToolCall) (llm.ToolResult, error) {
	start := time.Now()
	result := llm.ToolResult{
		ToolCallID: call.ID,
		Name:       call.Name,
	}

	// 1. 查找工具
	s, ok := e.registry.Get(call.Name)
	if !ok {
		err := fmt.Errorf("tool %s not found", call.Name)
		result.Error = err.Error()
		result.Success = false
		return result, err
	}

	// 2. 执行工具
	output, err := s.Execute(ctx, call.Arguments)
	result.DurationMs = time.Since(start).Milliseconds()

	if err != nil {
		result.Error = err.Error()
		result.Success = false
		return result, err
	}

	result.Output = string(output)
	result.Success = true
	return result, nil
}
