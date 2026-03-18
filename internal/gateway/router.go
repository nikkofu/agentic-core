package gateway

import (
	"agentic-core/internal/bus"
	"agentic-core/internal/logging"
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// Adapter 定义了对接外部通讯平台的标准接口
type Adapter interface {
	Name() string
	SendMessage(ctx context.Context, sessionID string, text string) error
}

type routeBinding struct {
	SessionID   string
	ChannelName string
	Metadata    map[string]interface{}
}

type gatewayResultEnvelope struct {
	Result  string           `json:"result,omitempty"`
	Message *ChannelResponse `json:"message,omitempty"`
}

// SessionRouter 负责将外部 IM 消息转换为内部任务并投递
type SessionRouter struct {
	queue    bus.TaskQueue
	adapters map[string]Adapter
	mu       sync.RWMutex
	routes   map[string]routeBinding
}

func NewSessionRouter(queue bus.TaskQueue) *SessionRouter {
	return &SessionRouter{
		queue:    queue,
		adapters: make(map[string]Adapter),
		routes:   make(map[string]routeBinding),
	}
}

func (r *SessionRouter) RegisterAdapter(adapter Adapter) {
	r.adapters[adapter.Name()] = adapter
}

// HandleIncoming 接收适配器解析后的消息，生成主控任务
func (r *SessionRouter) HandleIncoming(ctx context.Context, req ChannelRequest) error {
	_, err := r.DispatchIncoming(ctx, req)
	return err
}

func (r *SessionRouter) DispatchIncoming(ctx context.Context, req ChannelRequest) (string, error) {
	// 在 Agentic-Core 架构中，这里会根据 SessionID 判断是否已有活跃的专职 Agent。
	// 目前我们将请求统一发给 orchestrator 进行初次调度。

	taskID := fmt.Sprintf("session_%d", time.Now().UnixNano())

	// 封装为 Orchestrator 可以理解的内部任务
	payload, _ := json.Marshal(map[string]interface{}{
		"task":          req.Text,
		"channel":       req.ChannelName,
		"session_id":    req.SessionID,
		"message_id":    req.MessageID,
		"message_type":  req.MessageType,
		"format":        req.Format,
		"sender_id":     req.SenderID,
		"sender_name":   req.SenderName,
		"receiver_id":   req.ReceiverID,
		"parent_msg_id": req.ParentMessageID,
		"media":         req.Media,
		"articles":      req.Articles,
		"card":          req.Card,
		"raw_content":   req.RawContent,
		"metadata":      req.Metadata,
		"require_chat":  true, // 标记这是一个需要上下文聊天的任务
	})

	msg := bus.Message{
		MessageID:   taskID,
		SenderID:    "gateway." + req.ChannelName,
		ReceiverID:  "orchestrator",
		TargetAgent: "orchestrator", // 默认由主控接入
		Payload:     payload,
		Timestamp:   time.Now().UnixMilli(),
	}

	r.rememberRoute(taskID, req)

	// 投递到任务总线
	logging.Component("gateway").Info("incoming channel request",
		"sender_name", req.SenderName,
		"channel_name", req.ChannelName,
		"session_id", req.SessionID,
		"task_id", taskID,
	)
	if err := r.queue.Enqueue(ctx, "tasks", msg); err != nil {
		return "", err
	}
	return taskID, nil
}

// StartStreamListener 监听 Redis 上的任务结果，并通过适配器回推
func (r *SessionRouter) StartStreamListener(ctx context.Context) error {
	msgChan, err := r.queue.Dequeue(ctx, "task_results")
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
				var result bus.TaskResult
				if err := json.Unmarshal(msg.Payload, &result); err != nil {
					continue
				}

				resp, ok := decodeGatewayResponse(result.Output)
				if !ok {
					continue
				}

				binding, hasBinding := r.routeForTask(result.TaskID)
				if hasBinding {
					if resp.SessionID == "" {
						resp.SessionID = binding.SessionID
					}
					if resp.ChannelName == "" {
						resp.ChannelName = binding.ChannelName
					}
					resp.Metadata = mergeMetadata(binding.Metadata, resp.Metadata)
				}

				if resp.ChannelName == "" {
					if hasBinding {
						r.deleteRoute(result.TaskID)
					}
					continue
				}

				if adapter, ok := r.adapters[resp.ChannelName]; ok {
					if rich, richOK := adapter.(RichAdapter); richOK {
						_ = rich.Send(ctx, resp)
					} else {
						_ = adapter.SendMessage(ctx, resp.SessionID, resp.Text)
					}
				}
				if hasBinding {
					r.deleteRoute(result.TaskID)
				}
			}
		}
	}()

	return nil
}

func (r *SessionRouter) rememberRoute(taskID string, req ChannelRequest) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.routes[taskID] = routeBinding{
		SessionID:   req.SessionID,
		ChannelName: req.ChannelName,
		Metadata:    cloneMetadata(req.Metadata),
	}
}

func (r *SessionRouter) routeForTask(taskID string) (routeBinding, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	binding, ok := r.routes[taskID]
	return binding, ok
}

func (r *SessionRouter) deleteRoute(taskID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.routes, taskID)
}

func decodeGatewayResponse(output json.RawMessage) (ChannelResponse, bool) {
	var envelope gatewayResultEnvelope
	if err := json.Unmarshal(output, &envelope); err != nil {
		return ChannelResponse{}, false
	}
	if envelope.Message != nil {
		return *envelope.Message, true
	}
	if envelope.Result != "" {
		return ChannelResponse{
			MessageType: MessageTypeText,
			Format:      FormatPlainText,
			Text:        envelope.Result,
		}, true
	}
	return ChannelResponse{}, false
}

func mergeMetadata(base, override map[string]interface{}) map[string]interface{} {
	if len(base) == 0 && len(override) == 0 {
		return nil
	}
	merged := cloneMetadata(base)
	if merged == nil {
		merged = make(map[string]interface{}, len(override))
	}
	for key, value := range override {
		merged[key] = value
	}
	return merged
}

func cloneMetadata(src map[string]interface{}) map[string]interface{} {
	if len(src) == 0 {
		return nil
	}
	cloned := make(map[string]interface{}, len(src))
	for key, value := range src {
		cloned[key] = value
	}
	return cloned
}
