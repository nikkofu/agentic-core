package skill

import (
	"agentic-core/internal/sandbox"
	"context"
	"encoding/json"
	"fmt"
)

// WasmSkill 是一个基于 WebAssembly 的动态技能
type WasmSkill struct {
	name        string
	description string
	schema      string
	isWrite     bool
	wasmBytes   []byte
	executor    sandbox.SandboxExecutor
}

func NewWasmSkill(name, desc, schema string, isWrite bool, wasmBytes []byte, executor sandbox.SandboxExecutor) *WasmSkill {
	return &WasmSkill{
		name:        name,
		description: desc,
		schema:      schema,
		isWrite:     isWrite,
		wasmBytes:   wasmBytes,
		executor:    executor,
	}
}

func (s *WasmSkill) Name() string {
	return s.name
}

func (s *WasmSkill) Description() string {
	return s.description
}

func (s *WasmSkill) Schema() string {
	return s.schema
}

func (s *WasmSkill) IsWriteOperation() bool {
	return s.isWrite
}

func (s *WasmSkill) Execute(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
	// 将 JSON 参数作为字符串通过命令行或标准输入传递给 Wasm
	// 这里我们简化为作为第一个参数传递
	args := []string{string(params)}
	
	stdout, err := s.executor.ExecuteWasm(ctx, s.wasmBytes, args)
	if err != nil {
		return nil, fmt.Errorf("wasm execution failed: %w", err)
	}

	return json.RawMessage(stdout), nil
}
