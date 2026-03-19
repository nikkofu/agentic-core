package gateway

import (
	"agentic-core/internal/llm"
	"agentic-core/internal/memory"
	"agentic-core/internal/process"
	"agentic-core/internal/skill"
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type ChatCompletionsHandler struct {
	resolver     *llm.ModelResolver
	sender       *Sender
	auditor      *process.Auditor
	taskStore    memory.TaskStateStore
	executor     llm.ToolExecutor
	approvalGate llm.ApprovalGate
}

func NewChatCompletionsHandler(resolver *llm.ModelResolver, sender *Sender) *ChatCompletionsHandler {
	return NewChatCompletionsHandlerWithStore(resolver, sender, nil)
}

func NewChatCompletionsHandlerWithStore(resolver *llm.ModelResolver, sender *Sender, taskStore memory.TaskStateStore) *ChatCompletionsHandler {
	var auditor *process.Auditor
	if sender != nil {
		auditor = process.NewAuditor(sender.events, "gateway.chat")
	}
	return &ChatCompletionsHandler{
		resolver:  resolver,
		sender:    sender,
		auditor:   auditor,
		taskStore: taskStore,
	}
}

func NewChatCompletionsHandlerWithStoreRegistryAndApprovalGate(
	resolver *llm.ModelResolver,
	sender *Sender,
	taskStore memory.TaskStateStore,
	registry *skill.Registry,
	approvalGate llm.ApprovalGate,
) *ChatCompletionsHandler {
	handler := NewChatCompletionsHandlerWithStore(resolver, sender, taskStore)
	if registry != nil {
		handler.executor = skill.NewExecutor(registry)
	}
	handler.approvalGate = approvalGate
	return handler
}

func (h *ChatCompletionsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	raw, err := io.ReadAll(r.Body)
	if err != nil {
		h.writeError(w, 400, "invalid_request_error", err.Error())
		return
	}

	req, err := llm.ValidateChatCompletionRequest(raw)
	if err != nil {
		status, typ, msg := llm.MapProviderError(err)
		h.writeError(w, status, typ, msg)
		return
	}
	if req.Stream && len(req.Tools) > 0 {
		h.writeError(w, http.StatusBadRequest, "invalid_request_error", "tools are not yet supported on streaming gateway chat completions path")
		return
	}
	if len(req.Tools) > 0 && h.executor == nil {
		h.writeError(w, http.StatusBadRequest, "invalid_request_error", "tools are not yet supported on the gateway chat completions path")
		return
	}

	// 1. 解析模型别名路由
	provider, _, err := h.resolver.ResolveByAlias(req.Model)
	if err != nil {
		status, typ, msg := llm.MapProviderError(err)
		h.writeError(w, status, typ, msg)
		return
	}

	// 2. 构造内部推理请求
	traceID := fmt.Sprintf("tr_%d", time.Now().UnixNano())
	infReq := llm.InferenceRequest{
		TraceID:     traceID,
		TaskID:      traceID,
		Messages:    req.Messages,
		ModelAlias:  req.Model,
		Stream:      req.Stream,
		Temperature: req.Temperature,
	}
	h.recordRouteAudit(r.Context(), infReq)
	h.persistTaskState(r.Context(), infReq.TaskID, "gateway.chat", "running", "")

	if req.Stream {
		h.handleStream(r.Context(), w, provider, infReq)
	} else {
		h.handleNonStream(r.Context(), w, provider, infReq, len(req.Tools) > 0)
	}
}

func (h *ChatCompletionsHandler) handleNonStream(ctx context.Context, w http.ResponseWriter, p llm.Provider, req llm.InferenceRequest, toolMode bool) {
	if h.sender != nil || toolMode {
		taskID := req.TaskID
		if taskID == "" {
			taskID = req.TraceID
		}
		req.TaskID = taskID

		var fanout *llm.Fanout
		if h.sender != nil {
			fanout = llm.NewFanout(req.TraceID, req.SessionID, taskID)
			fanout.SetPublisher(h.sender.PublishChunk)
		}

		runtime := llm.NewRuntime(chatCompletionRuntimeProvider{
			provider:           p,
			passthroughActions: toolMode,
		}, h.executor, h.approvalGate)
		if !toolMode {
			runtime.MaxTurns = 1
		}

		result, err := runtime.Run(ctx, req, fanout)
		if err != nil {
			statusToPersist := result.Status
			if statusToPersist == "" {
				statusToPersist = "failed"
			}
			h.persistTaskState(ctx, req.TaskID, "gateway.chat", statusToPersist, err.Error())
			status, typ, msg := llm.MapProviderError(err)
			h.writeError(w, status, typ, msg)
			return
		}
		h.persistTaskState(ctx, req.TaskID, "gateway.chat", "success", "")
		h.writeNonStreamResponse(w, req, result.Content)
		return
	}

	content, err := p.Predict(ctx, req)
	if err != nil {
		h.persistTaskState(ctx, req.TaskID, "gateway.chat", "failed", err.Error())
		status, typ, msg := llm.MapProviderError(err)
		h.writeError(w, status, typ, msg)
		return
	}

	h.persistTaskState(ctx, req.TaskID, "gateway.chat", "success", "")
	h.writeNonStreamResponse(w, req, content)
}

func (h *ChatCompletionsHandler) writeNonStreamResponse(w http.ResponseWriter, req llm.InferenceRequest, content string) {
	resp := map[string]interface{}{
		"id":      req.TraceID,
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   req.ModelAlias,
		"choices": []map[string]interface{}{
			{
				"index": 0,
				"message": map[string]string{
					"role":    "assistant",
					"content": content,
				},
				"finish_reason": "stop",
			},
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (h *ChatCompletionsHandler) handleStream(ctx context.Context, w http.ResponseWriter, p llm.Provider, req llm.InferenceRequest) {
	SetSSEHeaders(w)

	if h.sender == nil {
		h.handleStreamPassthrough(ctx, w, p, req)
		return
	}

	taskID := req.TaskID
	if taskID == "" {
		taskID = req.TraceID
	}
	req.TaskID = taskID

	h.sender.RegisterSink(taskID, &ChatCompletionStreamSink{w: adaptResponseWriter(w)})
	defer h.sender.UnregisterSink(taskID)

	fanout := llm.NewFanout(req.TraceID, req.SessionID, taskID)
	fanout.SetPublisher(h.sender.PublishChunk)

	err := h.bridgeProviderStream(ctx, p, req, fanout)
	if err != nil {
		h.persistTaskState(ctx, req.TaskID, "gateway.chat", "failed", err.Error())
	} else {
		h.persistTaskState(ctx, req.TaskID, "gateway.chat", "success", "")
	}
	h.waitForSinkDrain(ctx, taskID)
}

func (h *ChatCompletionsHandler) handleStreamPassthrough(ctx context.Context, w http.ResponseWriter, p llm.Provider, req llm.InferenceRequest) {
	stream, err := p.PredictStream(ctx, req)
	if err != nil {
		data, _ := json.Marshal(map[string]interface{}{"error": err.Error()})
		_ = WriteSSEFrame(w, "error", data)
		return
	}
	defer stream.Close()

	_, _ = io.Copy(w, stream)
}

func (h *ChatCompletionsHandler) bridgeProviderStream(ctx context.Context, p llm.Provider, req llm.InferenceRequest, fanout *llm.Fanout) error {
	stream, err := p.PredictStream(ctx, req)
	if err != nil {
		if fanout != nil {
			_ = fanout.EmitError(ctx, err.Error())
		}
		return err
	}
	defer stream.Close()

	return publishProviderStream(ctx, stream, fanout)
}

func (h *ChatCompletionsHandler) waitForSinkDrain(ctx context.Context, taskID string) {
	if h == nil || h.sender == nil {
		return
	}

	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()

	for h.sender.HasSink(taskID) {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

type providerStreamEnvelope struct {
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
	} `json:"choices"`
}

func publishProviderStream(ctx context.Context, stream io.Reader, fanout *llm.Fanout) error {
	scanner := bufio.NewScanner(stream)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var (
		dataLines []string
		content   strings.Builder
	)

	flushFrame := func() error {
		if len(dataLines) == 0 {
			return nil
		}

		payload := strings.TrimSpace(strings.Join(dataLines, "\n"))
		dataLines = nil
		if payload == "" {
			return nil
		}

		if payload == "[DONE]" {
			if fanout != nil {
				return fanout.EmitFinal(ctx, content.String())
			}
			return nil
		}

		delta, streamErr, err := parseProviderStreamPayload([]byte(payload))
		if err != nil {
			if fanout != nil {
				_ = fanout.EmitError(ctx, err.Error())
			}
			return err
		}
		if streamErr != "" {
			if fanout != nil {
				_ = fanout.EmitError(ctx, streamErr)
			}
			return errors.New(streamErr)
		}

		if delta != "" {
			content.WriteString(delta)
		}
		if fanout != nil {
			chunk := fanout.NewChunk("delta")
			chunk.Role = "assistant"
			chunk.Delta = delta
			chunk.Data = json.RawMessage(payload)
			return fanout.Emit(ctx, chunk)
		}
		return nil
	}

	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			if fanout != nil {
				_ = fanout.EmitCancelled(ctx, err.Error())
			}
			return err
		}

		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			if err := flushFrame(); err != nil {
				return err
			}
			continue
		}
		if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}

	if err := scanner.Err(); err != nil {
		if fanout != nil {
			_ = fanout.EmitError(ctx, err.Error())
		}
		return err
	}

	if err := flushFrame(); err != nil {
		return err
	}

	if fanout != nil {
		return fanout.EmitFinal(ctx, content.String())
	}
	return nil
}

func parseProviderStreamPayload(raw []byte) (delta string, streamErr string, err error) {
	var payload providerStreamEnvelope
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", "", fmt.Errorf("invalid provider stream payload: %w", err)
	}
	if payload.Error.Message != "" {
		return "", payload.Error.Message, nil
	}
	if len(payload.Choices) == 0 {
		return "", "", nil
	}
	return payload.Choices[0].Delta.Content, "", nil
}

type chatCompletionRuntimeProvider struct {
	provider           llm.Provider
	passthroughActions bool
}

func (p chatCompletionRuntimeProvider) Predict(ctx context.Context, req llm.InferenceRequest) (string, error) {
	content, err := p.provider.Predict(ctx, req)
	if err != nil {
		return "", err
	}
	if p.passthroughActions {
		return content, nil
	}
	payload, err := json.Marshal(llm.ActionEnvelope{Final: content})
	if err != nil {
		return "", err
	}
	return string(payload), nil
}

func (p chatCompletionRuntimeProvider) PredictStream(ctx context.Context, req llm.InferenceRequest) (io.ReadCloser, error) {
	return p.provider.PredictStream(ctx, req)
}

func (h *ChatCompletionsHandler) recordRouteAudit(ctx context.Context, req llm.InferenceRequest) {
	if h == nil || h.auditor == nil {
		return
	}

	data, _ := json.Marshal(map[string]interface{}{
		"model_alias": req.ModelAlias,
		"stream":      req.Stream,
	})
	_ = h.auditor.Record(ctx, llm.AuditEvent{
		TraceID:     req.TraceID,
		SessionID:   req.SessionID,
		TaskID:      req.TaskID,
		Event:       "route",
		Actor:       "gateway.chat",
		Status:      "running",
		Data:        data,
		TimestampMs: time.Now().UnixMilli(),
	})
}

func (h *ChatCompletionsHandler) persistTaskState(ctx context.Context, taskID, agentName, status, errMsg string) {
	if h == nil || h.taskStore == nil || taskID == "" {
		return
	}
	_ = h.taskStore.Save(ctx, memory.TaskState{
		TaskID:        taskID,
		AgentName:     agentName,
		Status:        status,
		UpdatedAtUnix: time.Now().Unix(),
		ErrorMessage:  errMsg,
	})
}

func (h *ChatCompletionsHandler) writeError(w http.ResponseWriter, status int, typ, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"error": map[string]string{
			"type":    typ,
			"message": msg,
		},
	})
}

type ChatCompletionStreamSink struct {
	w httpResponseWriter
}

func (s *ChatCompletionStreamSink) WriteChunk(chunk llm.StreamChunk) error {
	switch chunk.Event {
	case "delta":
		if len(chunk.Data) > 0 {
			return WriteSSEFrame(s.w, "", chunk.Data)
		}
		data, _ := json.Marshal(map[string]string{"content": chunk.Delta})
		return WriteSSEFrame(s.w, "", data)
	case "final":
		return nil
	case "error", "timeout", "cancelled":
		data := chunk.Data
		if len(data) == 0 {
			data, _ = json.Marshal(map[string]string{"error": chunk.Error})
		}
		return WriteSSEFrame(s.w, "error", data)
	default:
		data, _ := json.Marshal(chunk)
		return WriteSSEFrame(s.w, chunk.Event, data)
	}
}

func (s *ChatCompletionStreamSink) Close() error {
	return WriteDoneFrame(s.w)
}

type responseWriterAdapter struct {
	http.ResponseWriter
}

func (w responseWriterAdapter) Flush() {
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func adaptResponseWriter(w http.ResponseWriter) httpResponseWriter {
	if rw, ok := w.(httpResponseWriter); ok {
		return rw
	}
	return responseWriterAdapter{ResponseWriter: w}
}
