package sandbox

import (
	"context"
	"fmt"
	"io"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

type SandboxExecutor interface {
	ExecuteWasm(ctx context.Context, wasmBytes []byte, args []string) ([]byte, error)
}

type WazeroExecutor struct {
	runtime wazero.Runtime
}

func NewWazeroExecutor() (*WazeroExecutor, error) {
	r := wazero.NewRuntime(context.Background())
	
	// 实例化 WASI
	if _, err := wasi_snapshot_preview1.Instantiate(context.Background(), r); err != nil {
		r.Close(context.Background())
		return nil, err
	}
	
	return &WazeroExecutor{runtime: r}, nil
}

func (e *WazeroExecutor) ExecuteWasm(ctx context.Context, wasmBytes []byte, args []string) (stdout []byte, err error) {
	// 创建内存中的 stdout 和 stderr
	stdoutReader, stdoutWriter := io.Pipe()
	stderrReader, stderrWriter := io.Pipe()

	// 使用 chan 接收结果
	stdoutChan := make(chan []byte, 1)
	go func() {
		data, _ := io.ReadAll(stdoutReader)
		stdoutChan <- data
	}()
	
	stderrChan := make(chan []byte, 1)
	go func() {
		data, _ := io.ReadAll(stderrReader)
		stderrChan <- data
	}()

	config := wazero.NewModuleConfig().
		WithStdout(stdoutWriter).
		WithStderr(stderrWriter).
		WithArgs(append([]string{"wasm_process"}, args...)...)

	// 编译并实例化模块
	mod, err := e.runtime.InstantiateWithConfig(ctx, wasmBytes, config)
	
	// 无论如何都要关闭写入端
	stdoutWriter.Close()
	stderrWriter.Close()

	if err != nil {
		return nil, fmt.Errorf("failed to instantiate wasm module: %w", err)
	}
	defer mod.Close(ctx)

	out := <-stdoutChan
	serr := <-stderrChan

	if len(serr) > 0 {
		return out, fmt.Errorf("wasm stderr: %s", string(serr))
	}

	return out, nil
}

func (e *WazeroExecutor) Close(ctx context.Context) error {
	return e.runtime.Close(ctx)
}
