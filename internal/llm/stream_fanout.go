package llm

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"time"
)

type ChunkPublisher func(ctx context.Context, chunk StreamChunk) error

// Fanout 负责管理 TraceID + TaskID 作用域内的序列号生成
type Fanout struct {
	TraceID      string
	SessionID    string
	TaskID       string
	sequence     int64
	terminalSent int32
	publisher    ChunkPublisher
}

func NewFanout(traceID, sessionID, taskID string) *Fanout {
	return &Fanout{
		TraceID:   traceID,
		SessionID: sessionID,
		TaskID:    taskID,
	}
}

func (f *Fanout) NextSequence() int64 {
	return atomic.AddInt64(&f.sequence, 1)
}

func (f *Fanout) SetPublisher(publisher ChunkPublisher) {
	f.publisher = publisher
}

func (f *Fanout) NewChunk(event string) StreamChunk {
	return StreamChunk{
		TraceID:     f.TraceID,
		SessionID:   f.SessionID,
		TaskID:      f.TaskID,
		Sequence:    f.NextSequence(),
		Event:       event,
		TimestampMs: time.Now().UnixMilli(),
	}
}

func (f *Fanout) Emit(ctx context.Context, chunk StreamChunk) error {
	if f == nil || f.publisher == nil {
		return nil
	}
	return f.publisher(ctx, chunk)
}

func (f *Fanout) EmitToolCall(ctx context.Context, call ToolCall) error {
	chunk := f.NewChunk("tool_call")
	chunk.ToolName = call.Name
	chunk.Role = "assistant"
	payload, err := json.Marshal(call)
	if err != nil {
		return err
	}
	chunk.Data = payload
	return f.Emit(ctx, chunk)
}

func (f *Fanout) EmitToolResult(ctx context.Context, result ToolResult) error {
	chunk := f.NewChunk("tool_result")
	chunk.ToolName = result.Name
	payload, err := json.Marshal(result)
	if err != nil {
		return err
	}
	chunk.Data = payload
	if !result.Success && result.Error != "" {
		chunk.Error = result.Error
	}
	return f.Emit(ctx, chunk)
}

func (f *Fanout) EmitWaitingApproval(ctx context.Context, req ApprovalRequest) error {
	chunk := f.NewChunk("waiting_approval")
	chunk.ToolName = req.ToolName
	payload, err := json.Marshal(req)
	if err != nil {
		return err
	}
	chunk.Data = payload
	return f.Emit(ctx, chunk)
}

func (f *Fanout) EmitFinal(ctx context.Context, content string) error {
	if f == nil {
		return nil
	}
	chunk := f.NewChunk("final")
	chunk.Role = "assistant"
	chunk.Done = true
	payload, err := json.Marshal(map[string]string{"content": content})
	if err != nil {
		return err
	}
	chunk.Data = payload
	return f.emitTerminal(ctx, chunk)
}

func (f *Fanout) EmitError(ctx context.Context, errMsg string) error {
	chunk := f.NewChunk("error")
	chunk.Done = true
	chunk.Error = errMsg
	return f.emitTerminal(ctx, chunk)
}

func (f *Fanout) EmitTimeout(ctx context.Context, errMsg string) error {
	chunk := f.NewChunk("timeout")
	chunk.Done = true
	chunk.Error = errMsg
	return f.emitTerminal(ctx, chunk)
}

func (f *Fanout) EmitCancelled(ctx context.Context, errMsg string) error {
	chunk := f.NewChunk("cancelled")
	chunk.Done = true
	chunk.Error = errMsg
	return f.emitTerminal(ctx, chunk)
}

func (f *Fanout) emitTerminal(ctx context.Context, chunk StreamChunk) error {
	if f == nil {
		return nil
	}
	if !atomic.CompareAndSwapInt32(&f.terminalSent, 0, 1) {
		return nil
	}
	return f.Emit(ctx, chunk)
}
