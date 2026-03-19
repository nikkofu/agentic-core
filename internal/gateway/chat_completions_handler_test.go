package gateway

import (
	"agentic-core/internal/bus"
	"agentic-core/internal/llm"
	"agentic-core/internal/memory"
	"agentic-core/internal/skill"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type handlerTestProvider struct {
	predictContent string
	predictOutputs []string
	predictCalls   int
	predictErr     error
	streamBody     io.ReadCloser
	streamErr      error
	lastRequest    llm.InferenceRequest
}

func (p *handlerTestProvider) Predict(ctx context.Context, req llm.InferenceRequest) (string, error) {
	p.lastRequest = req
	if p.predictErr != nil {
		return "", p.predictErr
	}
	if len(p.predictOutputs) > 0 {
		idx := p.predictCalls
		if idx >= len(p.predictOutputs) {
			idx = len(p.predictOutputs) - 1
		}
		p.predictCalls++
		return p.predictOutputs[idx], nil
	}
	return p.predictContent, nil
}

func (p *handlerTestProvider) PredictStream(ctx context.Context, req llm.InferenceRequest) (io.ReadCloser, error) {
	p.lastRequest = req
	if p.streamErr != nil {
		return nil, p.streamErr
	}
	return p.streamBody, nil
}

func newHandlerForTest(provider llm.Provider) *ChatCompletionsHandler {
	return newHandlerForTestWithSender(provider, nil)
}

func newHandlerForTestWithSender(provider llm.Provider, sender *Sender) *ChatCompletionsHandler {
	resolver := llm.NewModelResolver()
	resolver.Register("test-provider", provider)
	resolver.RegisterRoute(llm.StaticRoute{
		Alias:         "test-model",
		Provider:      "test-provider",
		UpstreamModel: "test-model-upstream",
	})
	return NewChatCompletionsHandler(resolver, sender)
}

func newHandlerForTestWithSenderAndRegistry(provider llm.Provider, sender *Sender, registry *skill.Registry) *ChatCompletionsHandler {
	return newHandlerForTestWithSenderAndRegistryAndStore(provider, sender, registry, nil)
}

func newHandlerForTestWithSenderAndRegistryAndStore(provider llm.Provider, sender *Sender, registry *skill.Registry, taskStore memory.TaskStateStore) *ChatCompletionsHandler {
	return newHandlerForTestWithSenderRegistryStoreAndApprovalGate(provider, sender, registry, taskStore, nil)
}

func newHandlerForTestWithSenderRegistryStoreAndApprovalGate(provider llm.Provider, sender *Sender, registry *skill.Registry, taskStore memory.TaskStateStore, approvalGate llm.ApprovalGate) *ChatCompletionsHandler {
	resolver := llm.NewModelResolver()
	resolver.Register("test-provider", provider)
	resolver.RegisterRoute(llm.StaticRoute{
		Alias:         "test-model",
		Provider:      "test-provider",
		UpstreamModel: "test-model-upstream",
	})
	return NewChatCompletionsHandlerWithStoreRegistryAndApprovalGate(resolver, sender, taskStore, registry, approvalGate)
}

type handlerTestWriteSkill struct{}

type handlerTestApprovalGate struct {
	wait func(ctx context.Context, req llm.ApprovalRequest, timeout time.Duration) (llm.ApprovalDecision, error)
}

func (g *handlerTestApprovalGate) WaitDecision(ctx context.Context, req llm.ApprovalRequest, timeout time.Duration) (llm.ApprovalDecision, error) {
	return g.wait(ctx, req, timeout)
}

func (s *handlerTestWriteSkill) Name() string {
	return "write_test_note"
}

func (s *handlerTestWriteSkill) Description() string {
	return "Persists a test note after approval."
}

func (s *handlerTestWriteSkill) Schema() string {
	return `{
		"type":"object",
		"properties":{"note":{"type":"string"}},
		"required":["note"]
	}`
}

func (s *handlerTestWriteSkill) IsWriteOperation() bool {
	return true
}

func (s *handlerTestWriteSkill) Execute(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
	return json.Marshal(map[string]any{
		"status": "written",
		"note":   "approved",
	})
}

func TestChatCompletionsHandlerRejectsUnknownField(t *testing.T) {
	handler := newHandlerForTest(&handlerTestProvider{predictContent: "ok"})

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions",
		strings.NewReader(`{"model":"test-model","messages":[{"role":"user","content":"hello"}],"extra":"x"}`))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}

	var payload map[string]map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if payload["error"]["type"] != "invalid_request_error" {
		t.Fatalf("expected invalid_request_error, got %+v", payload)
	}
}

func TestChatCompletionsHandlerRejectsInvalidToolChoice(t *testing.T) {
	handler := newHandlerForTest(&handlerTestProvider{predictContent: "ok"})

	body := `{
		"model":"test-model",
		"messages":[{"role":"user","content":"hello"}],
		"tools":[{"type":"function","function":{"name":"lookup"}}],
		"tool_choice":"sometimes"
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestChatCompletionsHandlerRejectsUnknownToolOnGatewayPath(t *testing.T) {
	provider := &handlerTestProvider{predictContent: "ok"}
	handler := newHandlerForTest(provider)

	body := `{
		"model":"test-model",
		"messages":[{"role":"user","content":"hello"}],
		"tools":[{"type":"function","function":{"name":"lookup"}}],
		"tool_choice":"auto"
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}

	var payload map[string]map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if payload["error"]["type"] != "invalid_request_error" {
		t.Fatalf("expected invalid_request_error, got %+v", payload)
	}
	if provider.lastRequest.ModelAlias != "" {
		t.Fatalf("expected provider not called, got %+v", provider.lastRequest)
	}
}

func TestChatCompletionsHandlerExecutesBuiltinToolRequestViaRuntime(t *testing.T) {
	transport := bus.NewFakeTransport()
	sender := NewSender(transport)

	chunks, err := transport.Subscribe(context.Background(), "chunks.*")
	if err != nil {
		t.Fatalf("subscribe chunks failed: %v", err)
	}

	provider := &handlerTestProvider{
		predictOutputs: []string{
			`{"think":"need time","call_skill":{"id":"call-1","name":"get_current_time","arguments":{}}}`,
			`{"think":"done","final":"The current time has been retrieved."}`,
		},
	}
	handler := newHandlerForTestWithSender(provider, sender)

	body := `{
		"model":"test-model",
		"messages":[{"role":"user","content":"what time is it?"}],
		"tools":[{"type":"function","function":{"name":"get_current_time"}}],
		"tool_choice":"required"
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	choices, ok := payload["choices"].([]any)
	if !ok || len(choices) != 1 {
		t.Fatalf("unexpected choices payload: %+v", payload)
	}
	choice, ok := choices[0].(map[string]any)
	if !ok {
		t.Fatalf("unexpected choice shape: %+v", choices[0])
	}
	message, ok := choice["message"].(map[string]any)
	if !ok {
		t.Fatalf("unexpected message shape: %+v", choice)
	}
	if message["content"] != "The current time has been retrieved." {
		t.Fatalf("unexpected final content: %+v", message["content"])
	}

	var events []string
	deadline := time.After(500 * time.Millisecond)
	for len(events) < 3 {
		select {
		case msg := <-chunks:
			var chunk llm.StreamChunk
			if err := json.Unmarshal(msg.Payload, &chunk); err != nil {
				t.Fatalf("unmarshal chunk failed: %v", err)
			}
			events = append(events, chunk.Event)
			if chunk.Done {
				break
			}
		case <-deadline:
			t.Fatalf("timed out waiting for tool runtime chunks, got %v", events)
		}
	}

	expected := []string{"tool_call", "tool_result", "final"}
	for idx, event := range expected {
		if events[idx] != event {
			t.Fatalf("expected events %v, got %v", expected, events)
		}
	}
	if provider.predictCalls != 2 {
		t.Fatalf("expected provider called twice through runtime loop, got %d", provider.predictCalls)
	}
}

func TestChatCompletionsHandlerExecutesWriteToolAfterApproval(t *testing.T) {
	transport := bus.NewFakeTransport()
	sender := NewSender(transport)

	chunks, err := transport.Subscribe(context.Background(), "chunks.*")
	if err != nil {
		t.Fatalf("subscribe chunks failed: %v", err)
	}

	registry := skill.NewRegistry()
	registry.Register(&handlerTestWriteSkill{})

	provider := &handlerTestProvider{
		predictOutputs: []string{
			`{"think":"need approval","call_skill":{"id":"call-1","name":"write_test_note","arguments":{"note":"hello"},"is_write_operation":true}}`,
			`{"think":"done","final":"Write approved and executed."}`,
		},
	}
	handler := newHandlerForTestWithSenderAndRegistry(provider, sender, registry)

	body := `{
		"model":"test-model",
		"messages":[{"role":"user","content":"persist this note"}],
		"tools":[{"type":"function","function":{"name":"write_test_note"}}],
		"tool_choice":"required"
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	rec := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		handler.ServeHTTP(rec, req)
		close(done)
	}()

	var events []string
	deadline := time.After(2 * time.Second)
	for len(events) < 4 {
		select {
		case msg := <-chunks:
			var chunk llm.StreamChunk
			if err := json.Unmarshal(msg.Payload, &chunk); err != nil {
				t.Fatalf("unmarshal chunk failed: %v", err)
			}
			events = append(events, chunk.Event)
			if chunk.Event != "waiting_approval" {
				continue
			}

			var approvalReq llm.ApprovalRequest
			if err := json.Unmarshal(chunk.Data, &approvalReq); err != nil {
				t.Fatalf("unmarshal approval request failed: %v", err)
			}
			if approvalReq.ToolName != "write_test_note" {
				t.Fatalf("unexpected approval request: %+v", approvalReq)
			}

			decision := llm.ApprovalDecision{
				TraceID:     chunk.TraceID,
				TaskID:      chunk.TaskID,
				ToolCallID:  approvalReq.ToolCallID,
				Approved:    true,
				Reviewer:    "tester",
				Reason:      "approved",
				DecidedAtMs: time.Now().UnixMilli(),
			}
			payload, err := json.Marshal(decision)
			if err != nil {
				t.Fatalf("marshal approval decision failed: %v", err)
			}
			if err := transport.Publish(context.Background(), "approvals", bus.Message{
				MessageID:  "approval." + decision.TaskID + "." + decision.ToolCallID,
				SenderID:   "test-reviewer",
				ReceiverID: "gateway",
				Payload:    payload,
				Timestamp:  decision.DecidedAtMs,
			}); err != nil {
				t.Fatalf("publish approval decision failed: %v", err)
			}
		case <-deadline:
			t.Fatalf("timed out waiting for approval flow chunks, got %v", events)
		}
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for handler completion")
	}

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Write approved and executed.") {
		t.Fatalf("unexpected response body: %s", rec.Body.String())
	}

	expected := []string{"tool_call", "waiting_approval", "tool_result", "final"}
	for idx, event := range expected {
		if events[idx] != event {
			t.Fatalf("expected events %v, got %v", expected, events)
		}
	}
	if provider.predictCalls != 2 {
		t.Fatalf("expected provider called twice through runtime loop, got %d", provider.predictCalls)
	}
	if len(provider.lastRequest.Messages) == 0 || provider.lastRequest.Messages[0].Role != "system" {
		t.Fatalf("expected system prompt in runtime request, got %+v", provider.lastRequest.Messages)
	}
	if !strings.Contains(provider.lastRequest.Messages[0].Content, "write_test_note") {
		t.Fatalf("expected system prompt to mention write_test_note, got %s", provider.lastRequest.Messages[0].Content)
	}
	if !strings.Contains(provider.lastRequest.Messages[0].Content, "is_write_operation=true") {
		t.Fatalf("expected system prompt to describe write operation, got %s", provider.lastRequest.Messages[0].Content)
	}
}

func TestChatCompletionsHandlerStreamsBuiltinToolRequestViaRuntime(t *testing.T) {
	transport := bus.NewFakeTransport()
	sender := NewSender(transport)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := sender.Start(ctx); err != nil {
		t.Fatalf("failed to start sender: %v", err)
	}

	provider := &handlerTestProvider{
		predictOutputs: []string{
			`{"think":"need time","call_skill":{"id":"call-1","name":"get_current_time","arguments":{}}}`,
			`{"think":"done","final":"The current time has been retrieved."}`,
		},
	}
	handler := newHandlerForTestWithSender(provider, sender)

	body := `{
		"model":"test-model",
		"messages":[{"role":"user","content":"what time is it?"}],
		"tools":[{"type":"function","function":{"name":"get_current_time"}}],
		"tool_choice":"required",
		"stream":true
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	bodyText := rec.Body.String()
	if !strings.Contains(bodyText, "event: tool_call") {
		t.Fatalf("expected tool_call event, got body:\n%s", bodyText)
	}
	if !strings.Contains(bodyText, "event: tool_result") {
		t.Fatalf("expected tool_result event, got body:\n%s", bodyText)
	}
	if strings.Count(bodyText, "[DONE]") != 1 {
		t.Fatalf("expected exactly one done marker, got body:\n%s", bodyText)
	}
	if provider.predictCalls != 2 {
		t.Fatalf("expected provider called twice through runtime loop, got %d", provider.predictCalls)
	}
}

func TestChatCompletionsHandlerStreamsWriteToolAfterApproval(t *testing.T) {
	transport := bus.NewFakeTransport()
	sender := NewSender(transport)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := sender.Start(ctx); err != nil {
		t.Fatalf("failed to start sender: %v", err)
	}

	chunks, err := transport.Subscribe(context.Background(), "chunks.*")
	if err != nil {
		t.Fatalf("subscribe chunks failed: %v", err)
	}

	registry := skill.NewRegistry()
	registry.Register(&handlerTestWriteSkill{})

	provider := &handlerTestProvider{
		predictOutputs: []string{
			`{"think":"need approval","call_skill":{"id":"call-1","name":"write_test_note","arguments":{"note":"hello"},"is_write_operation":true}}`,
			`{"think":"done","final":"Write approved and executed."}`,
		},
	}
	handler := newHandlerForTestWithSenderAndRegistry(provider, sender, registry)

	body := `{
		"model":"test-model",
		"messages":[{"role":"user","content":"persist this note"}],
		"tools":[{"type":"function","function":{"name":"write_test_note"}}],
		"tool_choice":"required",
		"stream":true
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	rec := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		handler.ServeHTTP(rec, req)
		close(done)
	}()

	var events []string
	deadline := time.After(2 * time.Second)
	for len(events) < 4 {
		select {
		case msg := <-chunks:
			var chunk llm.StreamChunk
			if err := json.Unmarshal(msg.Payload, &chunk); err != nil {
				t.Fatalf("unmarshal chunk failed: %v", err)
			}
			events = append(events, chunk.Event)
			if chunk.Event != "waiting_approval" {
				continue
			}

			var approvalReq llm.ApprovalRequest
			if err := json.Unmarshal(chunk.Data, &approvalReq); err != nil {
				t.Fatalf("unmarshal approval request failed: %v", err)
			}

			decision := llm.ApprovalDecision{
				TraceID:     chunk.TraceID,
				TaskID:      chunk.TaskID,
				ToolCallID:  approvalReq.ToolCallID,
				Approved:    true,
				Reviewer:    "tester",
				Reason:      "approved",
				DecidedAtMs: time.Now().UnixMilli(),
			}
			payload, err := json.Marshal(decision)
			if err != nil {
				t.Fatalf("marshal approval decision failed: %v", err)
			}
			if err := transport.Publish(context.Background(), "approvals", bus.Message{
				MessageID:  "approval." + decision.TaskID + "." + decision.ToolCallID,
				SenderID:   "test-reviewer",
				ReceiverID: "gateway",
				Payload:    payload,
				Timestamp:  decision.DecidedAtMs,
			}); err != nil {
				t.Fatalf("publish approval decision failed: %v", err)
			}
		case <-deadline:
			t.Fatalf("timed out waiting for approval flow chunks, got %v", events)
		}
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for handler completion")
	}

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	bodyText := rec.Body.String()
	if !strings.Contains(bodyText, "event: tool_call") {
		t.Fatalf("expected tool_call event, got body:\n%s", bodyText)
	}
	if !strings.Contains(bodyText, "event: waiting_approval") {
		t.Fatalf("expected waiting_approval event, got body:\n%s", bodyText)
	}
	if !strings.Contains(bodyText, "event: tool_result") {
		t.Fatalf("expected tool_result event, got body:\n%s", bodyText)
	}
	if strings.Count(bodyText, "[DONE]") != 1 {
		t.Fatalf("expected exactly one done marker, got body:\n%s", bodyText)
	}

	expected := []string{"tool_call", "waiting_approval", "tool_result", "final"}
	for idx, event := range expected {
		if events[idx] != event {
			t.Fatalf("expected events %v, got %v", expected, events)
		}
	}
	if provider.predictCalls != 2 {
		t.Fatalf("expected provider called twice through runtime loop, got %d", provider.predictCalls)
	}
}

func TestChatCompletionsHandlerRejectsWriteToolApprovalAndPersistsRejectedState(t *testing.T) {
	transport := bus.NewFakeTransport()
	sender := NewSender(transport)
	taskStore := memory.NewInMemoryTaskStateStore()

	chunks, err := transport.Subscribe(context.Background(), "chunks.*")
	if err != nil {
		t.Fatalf("subscribe chunks failed: %v", err)
	}
	audits, err := transport.Subscribe(context.Background(), "audit.*")
	if err != nil {
		t.Fatalf("subscribe audits failed: %v", err)
	}

	registry := skill.NewRegistry()
	registry.Register(&handlerTestWriteSkill{})

	provider := &handlerTestProvider{
		predictOutputs: []string{
			`{"think":"need approval","call_skill":{"id":"call-1","name":"write_test_note","arguments":{"note":"hello"},"is_write_operation":true}}`,
			`{"think":"done","final":"should not happen"}`,
		},
	}
	handler := newHandlerForTestWithSenderAndRegistryAndStore(provider, sender, registry, taskStore)

	body := `{
		"model":"test-model",
		"messages":[{"role":"user","content":"persist this note"}],
		"tools":[{"type":"function","function":{"name":"write_test_note"}}],
		"tool_choice":"required"
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	rec := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		handler.ServeHTTP(rec, req)
		close(done)
	}()

	var (
		taskID string
		events []string
	)
	deadline := time.After(2 * time.Second)
	for len(events) < 4 {
		select {
		case msg := <-chunks:
			var chunk llm.StreamChunk
			if err := json.Unmarshal(msg.Payload, &chunk); err != nil {
				t.Fatalf("unmarshal chunk failed: %v", err)
			}
			taskID = chunk.TaskID
			events = append(events, chunk.Event)
			if chunk.Event != "waiting_approval" {
				continue
			}

			var approvalReq llm.ApprovalRequest
			if err := json.Unmarshal(chunk.Data, &approvalReq); err != nil {
				t.Fatalf("unmarshal approval request failed: %v", err)
			}
			decision := llm.ApprovalDecision{
				TraceID:     chunk.TraceID,
				TaskID:      chunk.TaskID,
				ToolCallID:  approvalReq.ToolCallID,
				Approved:    false,
				Reviewer:    "tester",
				Reason:      "denied",
				DecidedAtMs: time.Now().UnixMilli(),
			}
			payload, err := json.Marshal(decision)
			if err != nil {
				t.Fatalf("marshal approval decision failed: %v", err)
			}
			if err := transport.Publish(context.Background(), "approvals", bus.Message{
				MessageID:  "approval." + decision.TaskID + "." + decision.ToolCallID,
				SenderID:   "test-reviewer",
				ReceiverID: "gateway",
				Payload:    payload,
				Timestamp:  decision.DecidedAtMs,
			}); err != nil {
				t.Fatalf("publish approval decision failed: %v", err)
			}
		case <-deadline:
			t.Fatalf("timed out waiting for rejection flow chunks, got %v", events)
		}
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for handler completion")
	}

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 rejection response, got %d body=%s", rec.Code, rec.Body.String())
	}
	var errorPayload map[string]map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &errorPayload); err != nil {
		t.Fatalf("decode rejection response failed: %v", err)
	}
	if errorPayload["error"]["type"] != "permission_error" {
		t.Fatalf("expected permission_error, got %+v", errorPayload)
	}
	expected := []string{"tool_call", "waiting_approval", "tool_result", "error"}
	for idx, event := range expected {
		if events[idx] != event {
			t.Fatalf("expected events %v, got %v", expected, events)
		}
	}
	if provider.predictCalls != 1 {
		t.Fatalf("expected provider called once on rejection, got %d", provider.predictCalls)
	}

	state, err := taskStore.Get(context.Background(), taskID)
	if err != nil {
		t.Fatalf("get task state failed: %v", err)
	}
	if state.Status != "rejected" {
		t.Fatalf("expected rejected task state, got %+v", state)
	}
	if !strings.Contains(state.ErrorMessage, "approval rejected") {
		t.Fatalf("expected rejection error message, got %+v", state)
	}

	var sawRoute, sawError bool
	auditDeadline := time.After(500 * time.Millisecond)
	for !(sawRoute && sawError) {
		select {
		case msg := <-audits:
			var event llm.AuditEvent
			if err := json.Unmarshal(msg.Payload, &event); err != nil {
				t.Fatalf("unmarshal audit event failed: %v", err)
			}
			if event.TaskID != taskID {
				continue
			}
			if event.Event == "route" {
				sawRoute = true
			}
			if event.Event == "error" && strings.Contains(event.Error, "approval rejected") {
				sawError = true
			}
		case <-auditDeadline:
			t.Fatalf("expected route and error audits for task %s", taskID)
		}
	}
}

func TestChatCompletionsHandlerTimeoutDuringApprovalPersistsTimeoutAndIgnoresLateDecision(t *testing.T) {
	transport := bus.NewFakeTransport()
	sender := NewSender(transport)
	taskStore := memory.NewInMemoryTaskStateStore()

	chunks, err := transport.Subscribe(context.Background(), "chunks.*")
	if err != nil {
		t.Fatalf("subscribe chunks failed: %v", err)
	}
	audits, err := transport.Subscribe(context.Background(), "audit.*")
	if err != nil {
		t.Fatalf("subscribe audits failed: %v", err)
	}

	registry := skill.NewRegistry()
	registry.Register(&handlerTestWriteSkill{})
	approvalGate := &handlerTestApprovalGate{
		wait: func(ctx context.Context, req llm.ApprovalRequest, timeout time.Duration) (llm.ApprovalDecision, error) {
			time.Sleep(20 * time.Millisecond)
			return llm.ApprovalDecision{}, context.DeadlineExceeded
		},
	}

	provider := &handlerTestProvider{
		predictOutputs: []string{
			`{"think":"need approval","call_skill":{"id":"call-1","name":"write_test_note","arguments":{"note":"hello"},"is_write_operation":true}}`,
		},
	}
	handler := newHandlerForTestWithSenderRegistryStoreAndApprovalGate(provider, sender, registry, taskStore, approvalGate)

	body := `{
		"model":"test-model",
		"messages":[{"role":"user","content":"persist this note"}],
		"tools":[{"type":"function","function":{"name":"write_test_note"}}],
		"tool_choice":"required"
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	rec := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		handler.ServeHTTP(rec, req)
		close(done)
	}()

	var (
		taskID string
		events []string
	)
	deadline := time.After(2 * time.Second)
	for len(events) < 3 {
		select {
		case msg := <-chunks:
			var chunk llm.StreamChunk
			if err := json.Unmarshal(msg.Payload, &chunk); err != nil {
				t.Fatalf("unmarshal chunk failed: %v", err)
			}
			taskID = chunk.TaskID
			events = append(events, chunk.Event)
			if chunk.Done {
				break
			}
		case <-deadline:
			t.Fatalf("timed out waiting for timeout flow chunks, got %v", events)
		}
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for handler completion")
	}

	if rec.Code != http.StatusGatewayTimeout {
		t.Fatalf("expected 504, got %d body=%s", rec.Code, rec.Body.String())
	}
	expected := []string{"tool_call", "waiting_approval", "timeout"}
	for idx, event := range expected {
		if events[idx] != event {
			t.Fatalf("expected events %v, got %v", expected, events)
		}
	}

	state, err := taskStore.Get(context.Background(), taskID)
	if err != nil {
		t.Fatalf("get task state failed: %v", err)
	}
	if state.Status != "timeout" {
		t.Fatalf("expected timeout task state, got %+v", state)
	}
	if !strings.Contains(state.ErrorMessage, "context deadline exceeded") {
		t.Fatalf("expected timeout error message, got %+v", state)
	}

	lateDecision := llm.ApprovalDecision{
		TraceID:     provider.lastRequest.TraceID,
		TaskID:      taskID,
		ToolCallID:  "call-1",
		Approved:    true,
		Reviewer:    "tester",
		Reason:      "late approval",
		DecidedAtMs: time.Now().UnixMilli(),
	}
	payload, err := json.Marshal(lateDecision)
	if err != nil {
		t.Fatalf("marshal late approval failed: %v", err)
	}
	if err := transport.Publish(context.Background(), "approvals", bus.Message{
		MessageID:  "approval." + lateDecision.TaskID + "." + lateDecision.ToolCallID + ".late",
		SenderID:   "test-reviewer",
		ReceiverID: "gateway",
		Payload:    payload,
		Timestamp:  lateDecision.DecidedAtMs,
	}); err != nil {
		t.Fatalf("publish late approval failed: %v", err)
	}

	stateAfterLateDecision, err := taskStore.Get(context.Background(), taskID)
	if err != nil {
		t.Fatalf("get task state after late approval failed: %v", err)
	}
	if stateAfterLateDecision.Status != "timeout" || stateAfterLateDecision.ErrorMessage != state.ErrorMessage {
		t.Fatalf("late approval changed task state: before=%+v after=%+v", state, stateAfterLateDecision)
	}

	select {
	case msg := <-chunks:
		var chunk llm.StreamChunk
		if err := json.Unmarshal(msg.Payload, &chunk); err != nil {
			t.Fatalf("unmarshal extra chunk failed: %v", err)
		}
		t.Fatalf("expected no extra chunk after late approval, got %+v", chunk)
	case <-time.After(100 * time.Millisecond):
	}

	var sawRoute, sawTimeout bool
	auditDeadline := time.After(500 * time.Millisecond)
	for !(sawRoute && sawTimeout) {
		select {
		case msg := <-audits:
			var event llm.AuditEvent
			if err := json.Unmarshal(msg.Payload, &event); err != nil {
				t.Fatalf("unmarshal audit event failed: %v", err)
			}
			if event.TaskID != taskID {
				continue
			}
			if event.Event == "route" {
				sawRoute = true
			}
			if event.Event == "timeout" {
				sawTimeout = true
			}
		case <-auditDeadline:
			t.Fatalf("expected route and timeout audits for task %s", taskID)
		}
	}
}

func TestChatCompletionsHandlerRejectsMissingModelAlias(t *testing.T) {
	resolver := llm.NewModelResolver()
	handler := NewChatCompletionsHandler(resolver, nil)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions",
		strings.NewReader(`{"model":"missing-model","messages":[{"role":"user","content":"hello"}]}`))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestChatCompletionsHandlerReturnsNonStreamResponse(t *testing.T) {
	provider := &handlerTestProvider{predictContent: "hello from provider"}
	handler := newHandlerForTest(provider)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions",
		strings.NewReader(`{"model":"test-model","messages":[{"role":"user","content":"hello"}]}`))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if payload["model"] != "test-model" {
		t.Fatalf("unexpected model: %+v", payload)
	}

	choices, ok := payload["choices"].([]interface{})
	if !ok || len(choices) != 1 {
		t.Fatalf("unexpected choices payload: %+v", payload)
	}

	if provider.lastRequest.ModelAlias != "test-model" {
		t.Fatalf("expected model alias test-model, got %s", provider.lastRequest.ModelAlias)
	}
	if len(provider.lastRequest.Messages) != 1 || provider.lastRequest.Messages[0].Content != "hello" {
		t.Fatalf("unexpected provider request: %+v", provider.lastRequest)
	}
}

func TestChatCompletionsHandlerNonStreamPublishesFinalChunkAndAudit(t *testing.T) {
	transport := bus.NewFakeTransport()
	sender := NewSender(transport)

	chunks, err := transport.Subscribe(context.Background(), "chunks.*")
	if err != nil {
		t.Fatalf("subscribe chunks failed: %v", err)
	}
	audits, err := transport.Subscribe(context.Background(), "audit.*")
	if err != nil {
		t.Fatalf("subscribe audit failed: %v", err)
	}

	provider := &handlerTestProvider{predictContent: "hello from provider"}
	handler := newHandlerForTestWithSender(provider, sender)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions",
		strings.NewReader(`{"model":"test-model","messages":[{"role":"user","content":"hello"}]}`))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	select {
	case msg := <-chunks:
		var chunk llm.StreamChunk
		if err := json.Unmarshal(msg.Payload, &chunk); err != nil {
			t.Fatalf("unmarshal chunk failed: %v", err)
		}
		if chunk.TaskID != provider.lastRequest.TraceID {
			t.Fatalf("expected task_id %s, got %s", provider.lastRequest.TraceID, chunk.TaskID)
		}
		if chunk.Event != "final" || !chunk.Done {
			t.Fatalf("expected terminal final chunk, got %+v", chunk)
		}

		var payload map[string]string
		if err := json.Unmarshal(chunk.Data, &payload); err != nil {
			t.Fatalf("unmarshal final payload failed: %v", err)
		}
		if payload["content"] != "hello from provider" {
			t.Fatalf("unexpected final payload: %+v", payload)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for non-stream final chunk")
	}

	deadline := time.After(500 * time.Millisecond)
	for {
		select {
		case msg := <-audits:
			var event llm.AuditEvent
			if err := json.Unmarshal(msg.Payload, &event); err != nil {
				t.Fatalf("unmarshal audit event failed: %v", err)
			}
			if event.Event != "final" {
				continue
			}
			if event.TaskID != provider.lastRequest.TraceID {
				t.Fatalf("expected audit task_id %s, got %s", provider.lastRequest.TraceID, event.TaskID)
			}
			return
		case <-deadline:
			t.Fatal("timed out waiting for non-stream final audit event")
		}
	}
}

func TestChatCompletionsHandlerNonStreamPublishesTimeoutChunkViaRuntime(t *testing.T) {
	transport := bus.NewFakeTransport()
	sender := NewSender(transport)

	chunks, err := transport.Subscribe(context.Background(), "chunks.*")
	if err != nil {
		t.Fatalf("subscribe chunks failed: %v", err)
	}

	provider := &handlerTestProvider{predictErr: context.DeadlineExceeded}
	handler := newHandlerForTestWithSender(provider, sender)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions",
		strings.NewReader(`{"model":"test-model","messages":[{"role":"user","content":"hello"}]}`))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusGatewayTimeout {
		t.Fatalf("expected 504, got %d", rec.Code)
	}

	select {
	case msg := <-chunks:
		var chunk llm.StreamChunk
		if err := json.Unmarshal(msg.Payload, &chunk); err != nil {
			t.Fatalf("unmarshal chunk failed: %v", err)
		}
		if chunk.Event != "timeout" || !chunk.Done {
			t.Fatalf("expected timeout terminal chunk, got %+v", chunk)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for timeout chunk")
	}
}

func TestChatCompletionsHandlerPublishesRouteAuditBeforeExecution(t *testing.T) {
	transport := bus.NewFakeTransport()
	sender := NewSender(transport)

	audits, err := transport.Subscribe(context.Background(), "audit.*")
	if err != nil {
		t.Fatalf("subscribe audit failed: %v", err)
	}

	provider := &handlerTestProvider{predictContent: "hello from provider"}
	handler := newHandlerForTestWithSender(provider, sender)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions",
		strings.NewReader(`{"model":"test-model","messages":[{"role":"user","content":"hello"}]}`))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	select {
	case msg := <-audits:
		var event llm.AuditEvent
		if err := json.Unmarshal(msg.Payload, &event); err != nil {
			t.Fatalf("unmarshal audit event failed: %v", err)
		}
		if event.Event != "route" {
			t.Fatalf("expected first audit event route, got %+v", event)
		}
		if event.Status != "running" {
			t.Fatalf("expected route audit status running, got %+v", event)
		}
		if event.TaskID != provider.lastRequest.TraceID {
			t.Fatalf("expected route audit task_id %s, got %s", provider.lastRequest.TraceID, event.TaskID)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for route audit event")
	}
}

func TestChatCompletionsHandlerStreamDoesNotDuplicateDoneFrame(t *testing.T) {
	provider := &handlerTestProvider{
		streamBody: io.NopCloser(strings.NewReader(
			"data: {\"id\":\"chunk-1\",\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\n" +
				"data: [DONE]\n\n",
		)),
	}
	handler := newHandlerForTest(provider)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions",
		strings.NewReader(`{"model":"test-model","messages":[{"role":"user","content":"hello"}],"stream":true}`))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if strings.Count(rec.Body.String(), "[DONE]") != 1 {
		t.Fatalf("expected exactly one done marker, got body:\n%s", rec.Body.String())
	}
	expected := "data: {\"id\":\"chunk-1\",\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\ndata: [DONE]\n\n"
	if rec.Body.String() != expected {
		t.Fatalf("expected stream body to pass through unchanged, got:\n%s", rec.Body.String())
	}
}

func TestChatCompletionsHandlerStreamPublishesUnifiedChunks(t *testing.T) {
	transport := bus.NewFakeTransport()
	sender := NewSender(transport)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := sender.Start(ctx); err != nil {
		t.Fatalf("failed to start sender: %v", err)
	}

	chunks, err := transport.Subscribe(ctx, "chunks.*")
	if err != nil {
		t.Fatalf("subscribe chunks failed: %v", err)
	}

	provider := &handlerTestProvider{
		streamBody: io.NopCloser(strings.NewReader(
			"data: {\"id\":\"chunk-1\",\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\n" +
				"data: [DONE]\n\n",
		)),
	}
	handler := newHandlerForTestWithSender(provider, sender)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions",
		strings.NewReader(`{"model":"test-model","messages":[{"role":"user","content":"hello"}],"stream":true}`))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var got []llm.StreamChunk
	deadline := time.After(500 * time.Millisecond)
	for len(got) < 2 {
		select {
		case msg := <-chunks:
			var chunk llm.StreamChunk
			if err := json.Unmarshal(msg.Payload, &chunk); err != nil {
				t.Fatalf("unmarshal chunk failed: %v", err)
			}
			got = append(got, chunk)
			if chunk.Done {
				break
			}
		case <-deadline:
			t.Fatalf("timed out waiting for unified chunks, got %d", len(got))
		}
	}

	if got[0].Event != "delta" || got[0].Delta != "hi" {
		t.Fatalf("expected first chunk to be delta=hi, got %+v", got[0])
	}
	if got[0].Sequence != 1 {
		t.Fatalf("expected first chunk sequence 1, got %d", got[0].Sequence)
	}

	if got[1].Event != "final" || !got[1].Done {
		t.Fatalf("expected second chunk to be terminal final, got %+v", got[1])
	}
	if got[1].Sequence != 2 {
		t.Fatalf("expected final chunk sequence 2, got %d", got[1].Sequence)
	}

	var finalPayload map[string]string
	if err := json.Unmarshal(got[1].Data, &finalPayload); err != nil {
		t.Fatalf("unmarshal final payload failed: %v", err)
	}
	if finalPayload["content"] != "hi" {
		t.Fatalf("expected final content hi, got %+v", finalPayload)
	}
}
