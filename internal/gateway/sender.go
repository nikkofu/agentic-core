package gateway

import (
	"agentic-core/internal/bus"
	"agentic-core/internal/llm"
	"agentic-core/internal/process"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// StreamSink 定义了 SSE 响应的接收端接口
type StreamSink interface {
	WriteChunk(chunk llm.StreamChunk) error
	Close() error
}

// Sender 负责管理全局流分发，将内部 Redis Chunk 路由到外部 HTTP/SSE
type Sender struct {
	mu      sync.RWMutex
	sinks   map[string]StreamSink // TaskID -> Sink
	events  bus.EventBus
	auditor *process.Auditor
}

func NewSender(events bus.EventBus) *Sender {
	return &Sender{
		sinks:   make(map[string]StreamSink),
		events:  events,
		auditor: process.NewAuditor(events, "gateway.sender"),
	}
}

// RegisterSink 注册一个任务的接收端
func (s *Sender) RegisterSink(taskID string, sink StreamSink) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sinks[taskID] = sink
}

// UnregisterSink 注销接收端
func (s *Sender) UnregisterSink(taskID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sinks, taskID)
}

func (s *Sender) HasSink(taskID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.sinks[taskID]
	return ok
}

func (s *Sender) PublishChunk(ctx context.Context, chunk llm.StreamChunk) error {
	if s == nil || s.events == nil {
		return nil
	}

	if chunk.TaskID == "" {
		return fmt.Errorf("task_id is required")
	}
	if chunk.TimestampMs == 0 {
		chunk.TimestampMs = time.Now().UnixMilli()
	}

	payload, err := json.Marshal(chunk)
	if err != nil {
		return err
	}

	if err := s.events.Publish(ctx, "chunks."+chunk.TaskID, bus.Message{
		MessageID:  fmt.Sprintf("chunk.%s.%d", chunk.TaskID, chunk.Sequence),
		SenderID:   "gateway.sender",
		ReceiverID: "sender",
		Payload:    payload,
		Timestamp:  chunk.TimestampMs,
	}); err != nil {
		return err
	}

	if s.auditor != nil {
		if err := s.auditor.Record(ctx, llm.AuditEvent{
			TraceID:     chunk.TraceID,
			SessionID:   chunk.SessionID,
			TaskID:      chunk.TaskID,
			Event:       chunk.Event,
			Actor:       "gateway.sender",
			Error:       chunk.Error,
			Data:        chunk.Data,
			TimestampMs: chunk.TimestampMs,
		}); err != nil {
			return err
		}
	}

	return nil
}

// Start 启动全局订阅循环
func (s *Sender) Start(ctx context.Context) error {
	// 订阅所有 chunks 事件
	msgChan, err := s.events.Subscribe(ctx, "chunks.*")
	if err != nil {
		return err
	}

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-msgChan:
				if !ok {
					return
				}
				var chunk llm.StreamChunk
				if err := json.Unmarshal(msg.Payload, &chunk); err != nil {
					continue
				}

				s.mu.RLock()
				sink, ok := s.sinks[chunk.TaskID]
				s.mu.RUnlock()

				if ok {
					if err := sink.WriteChunk(chunk); err != nil {
						// 如果写入失败（如客户端断连），注销 sink
						s.UnregisterSink(chunk.TaskID)
						s.recordAbortedStream(ctx, chunk, err)
						continue
					}
					if chunk.Done {
						s.UnregisterSink(chunk.TaskID)
						sink.Close()
					}
				}
			}
		}
	}()

	return nil
}

func (s *Sender) recordAbortedStream(ctx context.Context, chunk llm.StreamChunk, cause error) {
	if s == nil || s.auditor == nil {
		return
	}

	status := "aborted"
	if chunk.Done {
		status = "done"
	}

	data, _ := json.Marshal(map[string]interface{}{
		"stream_event": chunk.Event,
		"sequence":     chunk.Sequence,
		"done":         chunk.Done,
		"sink_error":   cause.Error(),
	})

	_ = s.auditor.Record(ctx, llm.AuditEvent{
		TraceID:     chunk.TraceID,
		SessionID:   chunk.SessionID,
		TaskID:      chunk.TaskID,
		Event:       "aborted_stream",
		Actor:       "gateway.sender",
		Status:      status,
		Error:       cause.Error(),
		Data:        data,
		TimestampMs: time.Now().UnixMilli(),
	})
}

// SSEStreamSink 实现 StreamSink 接口
type SSEStreamSink struct {
	w httpResponseWriter // 包装后的 ResponseWriter
}

type httpResponseWriter interface {
	Header() http.Header
	Write([]byte) (int, error)
	Flush()
}

func (s *SSEStreamSink) WriteChunk(chunk llm.StreamChunk) error {
	data, _ := json.Marshal(chunk)
	return WriteSSEFrame(s.w, chunk.Event, data)
}

func (s *SSEStreamSink) Close() error {
	return WriteDoneFrame(s.w)
}
