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
	resolver             *llm.ModelResolver
	sender               *Sender
	auditor              *process.Auditor
	taskStore            memory.TaskStateStore
	registry             *skill.Registry
	approvalGateOverride llm.ApprovalGate
}

func NewChatCompletionsHandler(resolver *llm.ModelResolver, sender *Sender) *ChatCompletionsHandler {
	return NewChatCompletionsHandlerWithStore(resolver, sender, nil)
}

func NewChatCompletionsHandlerWithStore(resolver *llm.ModelResolver, sender *Sender, taskStore memory.TaskStateStore) *ChatCompletionsHandler {
	return NewChatCompletionsHandlerWithStoreAndRegistry(resolver, sender, taskStore, nil)
}

func NewChatCompletionsHandlerWithStoreAndRegistry(resolver *llm.ModelResolver, sender *Sender, taskStore memory.TaskStateStore, registry *skill.Registry) *ChatCompletionsHandler {
	return NewChatCompletionsHandlerWithStoreRegistryAndApprovalGate(resolver, sender, taskStore, registry, nil)
}

func NewChatCompletionsHandlerWithStoreRegistryAndApprovalGate(resolver *llm.ModelResolver, sender *Sender, taskStore memory.TaskStateStore, registry *skill.Registry, approvalGate llm.ApprovalGate) *ChatCompletionsHandler {
	var auditor *process.Auditor
	if sender != nil {
		auditor = process.NewAuditor(sender.events, "gateway.chat")
	}
	if approvalGate == nil && sender != nil {
		approvalGate = skill.NewApprovalGate(sender.events)
	}
	return &ChatCompletionsHandler{
		resolver:             resolver,
		sender:               sender,
		auditor:              auditor,
		taskStore:            taskStore,
		registry:             registry,
		approvalGateOverride: approvalGate,
	}
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

	registry := h.skillRegistry()
	if len(req.Tools) > 0 {
		toolNames, err := parseRequestedToolNames(req.Tools)
		if err != nil {
			h.writeError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
			return
		}
		if err := ensureSupportedRequestedTools(registry, toolNames); err != nil {
			h.writeError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
			return
		}
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
	if len(req.Tools) > 0 {
		infReq.Metadata = map[string]string{
			"requested_tools": string(req.Tools),
			"tool_choice":     string(req.ToolChoice),
		}
		infReq.OnApprovalReject = "fail"
	}
	h.recordRouteAudit(r.Context(), infReq)
	h.persistTaskState(r.Context(), infReq.TaskID, "gateway.chat", "running", "")

	if req.Stream {
		h.handleStream(r.Context(), w, provider, infReq)
	} else {
		h.handleNonStream(r.Context(), w, provider, infReq)
	}
}

func (h *ChatCompletionsHandler) handleNonStream(ctx context.Context, w http.ResponseWriter, p llm.Provider, req llm.InferenceRequest) {
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

	if h.sender != nil || hasRequestedTools(req) {
		runtime := llm.NewRuntime(chatCompletionRuntimeProvider{provider: p, wrapFinal: true}, nil, nil)
		runtime.MaxTurns = 1
		if hasRequestedTools(req) {
			var err error
			req.Messages, err = buildGatewayToolRuntimeMessages(h.skillRegistry(), req.Messages, req.Metadata["requested_tools"], req.Metadata["tool_choice"])
			if err != nil {
				h.persistTaskState(ctx, req.TaskID, "gateway.chat", "failed", err.Error())
				h.writeError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
				return
			}
			executor := skill.NewExecutor(h.skillRegistry())
			runtime = llm.NewRuntime(chatCompletionRuntimeProvider{provider: p}, executor, h.approvalGate())
		}

		result, err := runtime.Run(ctx, req, fanout)
		if err != nil {
			h.persistTaskState(ctx, req.TaskID, "gateway.chat", taskStatusForResult(result, err), err.Error())
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
		h.persistTaskState(ctx, req.TaskID, "gateway.chat", taskStatusForError(err), err.Error())
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
	if hasRequestedTools(req) && h.sender == nil {
		h.writeError(w, http.StatusBadRequest, "invalid_request_error", "streaming tool execution requires gateway sender support")
		return
	}

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

	var err error
	if hasRequestedTools(req) {
		req.Messages, err = buildGatewayToolRuntimeMessages(h.skillRegistry(), req.Messages, req.Metadata["requested_tools"], req.Metadata["tool_choice"])
		if err != nil {
			h.persistTaskState(ctx, req.TaskID, "gateway.chat", "failed", err.Error())
			data, _ := json.Marshal(map[string]string{"error": err.Error()})
			_ = WriteSSEFrame(adaptResponseWriter(w), "error", data)
			_ = WriteDoneFrame(adaptResponseWriter(w))
			return
		}
		executor := skill.NewExecutor(h.skillRegistry())
		runtime := llm.NewRuntime(chatCompletionRuntimeProvider{provider: p}, executor, h.approvalGate())
		result, runErr := runtime.Run(ctx, req, fanout)
		err = runErr
		if err != nil {
			h.persistTaskState(ctx, req.TaskID, "gateway.chat", taskStatusForResult(result, err), err.Error())
		}
	} else {
		err = h.bridgeProviderStream(ctx, p, req, fanout)
	}
	if err != nil {
		if hasRequestedTools(req) {
			h.waitForSinkDrain(ctx, taskID)
			return
		}
		h.persistTaskState(ctx, req.TaskID, "gateway.chat", taskStatusForError(err), err.Error())
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
	provider  llm.Provider
	wrapFinal bool
}

func (p chatCompletionRuntimeProvider) Predict(ctx context.Context, req llm.InferenceRequest) (string, error) {
	content, err := p.provider.Predict(ctx, req)
	if err != nil {
		return "", err
	}
	if p.wrapFinal {
		payload, err := json.Marshal(llm.ActionEnvelope{Final: content})
		if err != nil {
			return "", err
		}
		return string(payload), nil
	}
	return content, nil
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

type requestedFunctionTool struct {
	Type     string `json:"type"`
	Function struct {
		Name string `json:"name"`
	} `json:"function"`
}

func parseRequestedToolNames(raw json.RawMessage) ([]string, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var tools []requestedFunctionTool
	if err := json.Unmarshal(raw, &tools); err != nil {
		return nil, fmt.Errorf("invalid tools payload: %w", err)
	}
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Function.Name)
	}
	return names, nil
}

func ensureSupportedRequestedTools(registry *skill.Registry, names []string) error {
	for _, name := range names {
		if _, ok := registry.Get(name); !ok {
			return fmt.Errorf("tool %s is not supported on the gateway chat completions path", name)
		}
	}
	return nil
}

func newGatewaySkillRegistry() *skill.Registry {
	registry := skill.NewRegistry()
	registry.Register(&skill.CurrentTimeSkill{})
	registry.Register(&skill.HttpGetSkill{})
	return registry
}

func (h *ChatCompletionsHandler) skillRegistry() *skill.Registry {
	if h != nil && h.registry != nil {
		return h.registry
	}
	return newGatewaySkillRegistry()
}

func (h *ChatCompletionsHandler) approvalGate() llm.ApprovalGate {
	if h != nil && h.approvalGateOverride != nil {
		return h.approvalGateOverride
	}
	if h != nil && h.sender != nil {
		return skill.NewApprovalGate(h.sender.events)
	}
	return nil
}

func hasRequestedTools(req llm.InferenceRequest) bool {
	return req.Metadata != nil && strings.TrimSpace(req.Metadata["requested_tools"]) != ""
}

func buildGatewayToolRuntimeMessages(registry *skill.Registry, messages []llm.ChatMessage, requestedToolsRaw string, toolChoiceRaw string) ([]llm.ChatMessage, error) {
	if registry == nil {
		registry = newGatewaySkillRegistry()
	}
	toolNames, err := parseRequestedToolNames(json.RawMessage(requestedToolsRaw))
	if err != nil {
		return nil, err
	}

	builder := llm.NewSystemPromptBuilder("gateway.chat")
	builder.AddInstruction("You are executing a chat completion request with gateway-managed tools.")
	builder.AddInstruction("You must respond with a JSON object matching exactly one of: {\"think\":\"...\",\"call_skill\":{\"id\":\"...\",\"name\":\"...\",\"arguments\":{...},\"is_write_operation\":<tool-required-bool>}} or {\"think\":\"...\",\"final\":\"...\"}.")
	builder.AddInstruction("When calling a tool, use only the provided builtin tool names and JSON object arguments.")
	if toolChoiceRaw != "" && toolChoiceRaw != "\"auto\"" {
		builder.AddInstruction("Respect the caller's tool_choice constraint while staying within the available builtin tools.")
	}

	for _, name := range toolNames {
		toolDef, ok := registry.Get(name)
		if !ok {
			return nil, fmt.Errorf("tool %s is not supported on the gateway chat completions path", name)
		}
		builder.AddInstruction(fmt.Sprintf("Tool %s requires is_write_operation=%t.", toolDef.Name(), toolDef.IsWriteOperation()))
		builder.AddTool(llm.ToolInfo{
			Name:        toolDef.Name(),
			Description: toolDef.Description(),
			Params:      toolDef.Schema(),
		})
	}

	prompt := builder.Build()
	return append([]llm.ChatMessage{{
		Role:    "system",
		Content: prompt,
	}}, messages...), nil
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

func taskStatusForResult(result llm.FinalResult, err error) string {
	if result.Status != "" {
		return result.Status
	}
	return taskStatusForError(err)
}

func taskStatusForError(err error) string {
	if err == nil {
		return "success"
	}
	errLower := strings.ToLower(err.Error())
	switch {
	case errors.Is(err, context.Canceled):
		return "cancelled"
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case strings.Contains(errLower, "approval rejected"):
		return "rejected"
	case strings.Contains(errLower, "timeout"):
		return "timeout"
	default:
		return "failed"
	}
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
