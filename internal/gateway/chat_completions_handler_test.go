package gateway

import (
	"agentic-core/internal/bus"
	"agentic-core/internal/llm"
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

func TestChatCompletionsHandlerRejectsToolsUntilRuntimeToolPathExists(t *testing.T) {
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
