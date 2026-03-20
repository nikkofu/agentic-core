package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"agentic-core/internal/bus"
	"agentic-core/internal/llm"
	"agentic-core/internal/memory"
	"agentic-core/internal/skill"
	"agentic-core/internal/workflow"
)

func TestParseAppConfigDefaultsSQLiteDSN(t *testing.T) {
	cfg, err := ParseAppConfig(nil)
	if err != nil {
		t.Fatalf("parse app config failed: %v", err)
	}
	if cfg.SQLiteDSN != "agentic_core.db" {
		t.Fatalf("expected default sqlite dsn, got %s", cfg.SQLiteDSN)
	}
}

func TestParseAppConfigAcceptsCustomSQLiteDSN(t *testing.T) {
	cfg, err := ParseAppConfig([]string{"--sqlite-dsn", ":memory:"})
	if err != nil {
		t.Fatalf("parse app config failed: %v", err)
	}
	if cfg.SQLiteDSN != ":memory:" {
		t.Fatalf("expected custom sqlite dsn, got %s", cfg.SQLiteDSN)
	}
}

func TestNewAppInitializesCoreComponents(t *testing.T) {
	cfg := AppConfig{SQLiteDSN: ":memory:", RedisAddr: "skip", MQTTBroker: "skip"}
	app, err := NewApp(context.Background(), cfg)
	if err != nil {
		t.Fatalf("new app failed: %v", err)
	}
	if app == nil {
		t.Fatal("expected app instance")
	}
	if app.taskStore == nil {
		t.Fatal("expected task store to be initialized")
	}
	if app.queue == nil {
		t.Fatal("expected queue to be initialized")
	}
	if app.events == nil {
		t.Fatal("expected event bus to be initialized")
	}
	if app.heartbeat == nil {
		t.Fatal("expected heartbeat bus to be initialized")
	}
	if app.logger == nil {
		t.Fatal("expected logger to be initialized")
	}
}

func TestAppRunReturnsContextErrorWhenCanceled(t *testing.T) {
	cfg := AppConfig{SQLiteDSN: ":memory:", RedisAddr: "skip", MQTTBroker: "skip"}
	app, err := NewApp(context.Background(), cfg)
	if err != nil {
		t.Fatalf("new app failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := app.Run(ctx); err != context.Canceled {
		t.Fatalf("expected context canceled, got %v", err)
	}
}

type httpChatProviderStub struct {
	predictContent string
	predictErr     error
}

func (p *httpChatProviderStub) Predict(ctx context.Context, req llm.InferenceRequest) (string, error) {
	if p.predictErr != nil {
		return "", p.predictErr
	}
	return p.predictContent, nil
}

func (p *httpChatProviderStub) PredictStream(ctx context.Context, req llm.InferenceRequest) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}

type orchestratorScriptedProvider struct {
	predictOutputs []string
	predictCalls   int
	predictErr     error
	lastRequest    llm.InferenceRequest
}

func (p *orchestratorScriptedProvider) Predict(ctx context.Context, req llm.InferenceRequest) (string, error) {
	p.lastRequest = req
	if p.predictErr != nil {
		return "", p.predictErr
	}
	idx := p.predictCalls
	if idx >= len(p.predictOutputs) {
		idx = len(p.predictOutputs) - 1
	}
	p.predictCalls++
	return p.predictOutputs[idx], nil
}

func (p *orchestratorScriptedProvider) PredictStream(ctx context.Context, req llm.InferenceRequest) (io.ReadCloser, error) {
	p.lastRequest = req
	return io.NopCloser(strings.NewReader("")), nil
}

type orchestratorTestWriteSkill struct{}

func (s *orchestratorTestWriteSkill) Name() string {
	return "write_test_note"
}

func (s *orchestratorTestWriteSkill) Description() string {
	return "Persists a test note after approval."
}

func (s *orchestratorTestWriteSkill) Schema() string {
	return `{"type":"object","properties":{"note":{"type":"string"}},"required":["note"]}`
}

func (s *orchestratorTestWriteSkill) IsWriteOperation() bool {
	return true
}

func (s *orchestratorTestWriteSkill) Execute(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
	return json.Marshal(map[string]any{
		"status": "written",
		"note":   "approved",
	})
}

type orchestratorTestApprovalGate struct {
	wait func(ctx context.Context, req llm.ApprovalRequest, timeout time.Duration) (llm.ApprovalDecision, error)
}

func (g *orchestratorTestApprovalGate) WaitDecision(ctx context.Context, req llm.ApprovalRequest, timeout time.Duration) (llm.ApprovalDecision, error) {
	return g.wait(ctx, req, timeout)
}

func newSignedApprovalWebhookRequest(t *testing.T, secret string, decision llm.ApprovalDecision) *http.Request {
	t.Helper()

	body, err := json.Marshal(decision)
	if err != nil {
		t.Fatalf("marshal approval decision failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/approval", strings.NewReader(string(body)))
	ts := time.Now().Unix()
	nonce := fmt.Sprintf("nonce-%d", time.Now().UnixNano())
	req.Header.Set(skill.HeaderTimestamp, fmt.Sprintf("%d", ts))
	req.Header.Set(skill.HeaderNonce, nonce)
	req.Header.Set(skill.HeaderSignature, skill.GenerateSignature(secret, ts, nonce, body))
	return req
}

func TestAppRunPerformsBusHealthPublishConsume(t *testing.T) {
	cfg := AppConfig{SQLiteDSN: ":memory:", RedisAddr: "skip", MQTTBroker: "skip"}
	app, err := NewApp(context.Background(), cfg)
	if err != nil {
		t.Fatalf("new app failed: %v", err)
	}

	ch, err := app.events.Subscribe(context.Background(), "system.health")
	if err != nil {
		t.Fatalf("expected health subscription, got %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	if err := app.Run(ctx); err != nil && err != context.DeadlineExceeded {
		t.Fatalf("expected run success or timeout, got %v", err)
	}

	select {
	case msg := <-ch:
		if msg.MessageID != "orchestrator.health" {
			t.Fatalf("unexpected health message id: %s", msg.MessageID)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for health message")
	}

	status, ok := app.heartbeat.(*bus.FakeHeartbeatBus).LastStatus("orchestrator")
	if !ok || status != "running" {
		t.Fatalf("expected orchestrator running heartbeat, got %v %v", status, ok)
	}
}

func TestProcessOneTaskSpawnsWorkflow(t *testing.T) {
	cfg := AppConfig{SQLiteDSN: ":memory:", RedisAddr: "skip", MQTTBroker: "skip"}
	app, err := NewApp(context.Background(), cfg)
	if err != nil {
		t.Fatalf("new app failed: %v", err)
	}

	msg := bus.Message{
		MessageID:  "task-1",
		SenderID:   "user",
		ReceiverID: "orchestrator",
		Payload:    json.RawMessage(`{"task":"demo"}`),
		Timestamp:  time.Now().UnixMilli(),
	}
	if err := app.queue.Enqueue(context.Background(), "tasks", msg); err != nil {
		t.Fatalf("enqueue task failed: %v", err)
	}

	// We don't want to actually spawn processes in tests
	app.proc = &fakeProcessManager{}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	// Start processing in a goroutine because ProcessOneTask will block on channel
	go func() {
		_ = app.ProcessOneTask(ctx)
	}()

	// Wait for workflow to be registered
	time.Sleep(200 * time.Millisecond)

	app.mu.RLock()
	_, exists := app.workflows["task-1"]
	app.mu.RUnlock()

	if !exists {
		t.Fatal("expected workflow to be created for task-1")
	}
}

func TestServeHTTPStoresChatCompletionSuccessState(t *testing.T) {
	cfg := AppConfig{SQLiteDSN: "file:orchestrator-http-success?mode=memory&cache=shared", RedisAddr: "skip", MQTTBroker: "skip"}
	app, err := NewApp(context.Background(), cfg)
	if err != nil {
		t.Fatalf("new app failed: %v", err)
	}

	provider := &httpChatProviderStub{predictContent: "hello from provider"}
	app.resolver.Register("test-provider", provider)
	app.resolver.RegisterRoute(llm.StaticRoute{
		Alias:         "test-model",
		Provider:      "test-provider",
		UpstreamModel: "test-model-upstream",
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions",
		strings.NewReader(`{"model":"test-model","messages":[{"role":"user","content":"hello"}]}`))
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	taskID, _ := payload["id"].(string)
	if taskID == "" {
		t.Fatalf("expected response id, got %+v", payload)
	}

	state, err := app.taskStore.Get(context.Background(), taskID)
	if err != nil {
		t.Fatalf("expected task state persisted, got %v", err)
	}
	if state.Status != "success" {
		t.Fatalf("expected success status, got %s", state.Status)
	}
}

func TestProcessOneTaskPersistsRunningTaskState(t *testing.T) {
	cfg := AppConfig{SQLiteDSN: "file:orchestrator-running?mode=memory&cache=shared", RedisAddr: "skip", MQTTBroker: "skip"}
	app, err := NewApp(context.Background(), cfg)
	if err != nil {
		t.Fatalf("new app failed: %v", err)
	}

	msg := bus.Message{
		MessageID:  "task-running",
		SenderID:   "user",
		ReceiverID: "orchestrator",
		Payload:    json.RawMessage(`{"task":"demo"}`),
		Timestamp:  time.Now().UnixMilli(),
	}
	if err := app.queue.Enqueue(context.Background(), "tasks", msg); err != nil {
		t.Fatalf("enqueue task failed: %v", err)
	}

	app.proc = &fakeProcessManager{}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := app.ProcessOneTask(ctx); err != nil {
		t.Fatalf("ProcessOneTask failed: %v", err)
	}

	state, err := app.taskStore.Get(context.Background(), "task-running")
	if err != nil {
		t.Fatalf("expected running task state, got error: %v", err)
	}
	if state.Status != "running" {
		t.Fatalf("expected running status, got %s", state.Status)
	}
	if state.AgentName != "orchestrator" {
		t.Fatalf("expected default agent name orchestrator, got %s", state.AgentName)
	}
}

func TestProcessOneTaskPublishesRouteAuditEvent(t *testing.T) {
	cfg := AppConfig{SQLiteDSN: "file:orchestrator-route?mode=memory&cache=shared", RedisAddr: "skip", MQTTBroker: "skip"}
	app, err := NewApp(context.Background(), cfg)
	if err != nil {
		t.Fatalf("new app failed: %v", err)
	}

	auditEvents, err := app.events.Subscribe(context.Background(), "audit.task-route")
	if err != nil {
		t.Fatalf("subscribe audit failed: %v", err)
	}

	msg := bus.Message{
		MessageID:  "task-route",
		SenderID:   "user",
		ReceiverID: "orchestrator",
		Payload:    json.RawMessage(`{"task":"demo"}`),
		Timestamp:  time.Now().UnixMilli(),
	}
	if err := app.queue.Enqueue(context.Background(), "tasks", msg); err != nil {
		t.Fatalf("enqueue task failed: %v", err)
	}

	app.proc = &fakeProcessManager{}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := app.ProcessOneTask(ctx); err != nil {
		t.Fatalf("ProcessOneTask failed: %v", err)
	}

	select {
	case msg := <-auditEvents:
		var event llm.AuditEvent
		if err := json.Unmarshal(msg.Payload, &event); err != nil {
			t.Fatalf("unmarshal audit event failed: %v", err)
		}
		if event.TaskID != "task-route" || event.Event != "route" {
			t.Fatalf("unexpected audit event: %+v", event)
		}
		if event.Actor != "orchestrator" {
			t.Fatalf("expected actor orchestrator, got %s", event.Actor)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("timed out waiting for route audit event")
	}
}

func TestListenResultsNormalizesFailedStatusesInStoreAndWorkflow(t *testing.T) {
	cfg := AppConfig{SQLiteDSN: "file:orchestrator-results?mode=memory&cache=shared", RedisAddr: "skip", MQTTBroker: "skip"}
	app, err := NewApp(context.Background(), cfg)
	if err != nil {
		t.Fatalf("new app failed: %v", err)
	}

	wf := workflow.NewWorkflow(func(ctx context.Context, agentType string, nodeID string) error {
		return nil
	})
	if err := wf.AddTask("task-failed", "researcher", nil); err != nil {
		t.Fatalf("add task failed: %v", err)
	}
	if err := wf.Start(context.Background()); err != nil {
		t.Fatalf("start workflow failed: %v", err)
	}

	app.mu.Lock()
	app.workflows["task-failed"] = wf
	app.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- app.ListenResults(ctx)
	}()

	result := bus.TaskResult{
		TaskID:    "task-failed",
		AgentName: "researcher",
		Status:    "error",
		Error:     "tool crashed",
		Timestamp: time.Now().Unix(),
		Output:    json.RawMessage(`{"result":""}`),
	}
	payload, _ := json.Marshal(result)
	if err := app.queue.Enqueue(context.Background(), "task_results", bus.Message{
		MessageID:  "task-failed.result",
		SenderID:   "task-failed",
		ReceiverID: "orchestrator",
		Payload:    payload,
		Timestamp:  time.Now().UnixMilli(),
	}); err != nil {
		t.Fatalf("enqueue result failed: %v", err)
	}

	var state memory.TaskState
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		state, err = app.taskStore.Get(context.Background(), "task-failed")
		if err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("expected task state saved, got %v", err)
	}
	if state.Status != "failed" {
		t.Fatalf("expected normalized failed status, got %s", state.Status)
	}
	if state.ErrorMessage != "tool crashed" {
		t.Fatalf("expected error message preserved, got %s", state.ErrorMessage)
	}

	nodeState, ok := wf.NodeState("task-failed")
	if !ok {
		t.Fatal("expected workflow node state")
	}
	if nodeState != workflow.NodeStateFailed {
		t.Fatalf("expected workflow node failed, got %s", nodeState)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil && err != context.Canceled {
			t.Fatalf("unexpected ListenResults error: %v", err)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("timed out waiting for ListenResults to exit")
	}
}

func TestListenResultsPreservesTimeoutTaskState(t *testing.T) {
	status := memory.TaskStatusTimeout
	runListenResultsTerminalStateTest(t,
		"file:orchestrator-timeout?mode=memory&cache=shared",
		bus.TaskResult{
			TaskID:    "task-timeout",
			AgentName: "planner",
			Status:    status,
			Error:     "timeout waiting for subagent",
			Timestamp: time.Now().Unix(),
			Output:    json.RawMessage(`{"result":""}`),
		},
		memory.NormalizeTaskStatus(status),
	)
}

func TestListenResultsPreservesRejectedTaskState(t *testing.T) {
	status := memory.TaskStatusRejected
	runListenResultsTerminalStateTest(t,
		"file:orchestrator-rejected?mode=memory&cache=shared",
		bus.TaskResult{
			TaskID:    "task-rejected",
			AgentName: "policy",
			Status:    status,
			Error:     "governance rejected execution",
			Timestamp: time.Now().Unix(),
			Output:    json.RawMessage(`{"result":""}`),
		},
		memory.NormalizeTaskStatus(status),
	)
}

func TestListenResultsPreservesCancelledTaskState(t *testing.T) {
	status := memory.TaskStatusCancelled
	runListenResultsTerminalStateTest(t,
		"file:orchestrator-cancelled?mode=memory&cache=shared",
		bus.TaskResult{
			TaskID:    "task-cancelled",
			AgentName: "planner",
			Status:    status,
			Error:     "task cancelled by request",
			Timestamp: time.Now().Unix(),
			Output:    json.RawMessage(`{"result":""}`),
		},
		memory.NormalizeTaskStatus(status),
	)
}

func TestListenResultsNormalizesUnknownTaskStateToFailed(t *testing.T) {
	status := "governance.rejected"
	runListenResultsTerminalStateTest(t,
		"file:orchestrator-unknown?mode=memory&cache=shared",
		bus.TaskResult{
			TaskID:    "task-unknown",
			AgentName: "policy",
			Status:    status,
			Error:     "unknown governance state",
			Timestamp: time.Now().Unix(),
			Output:    json.RawMessage(`{"result":""}`),
		},
		memory.NormalizeTaskStatus(status),
	)
}

func runListenResultsTerminalStateTest(t *testing.T, sqliteDSN string, result bus.TaskResult, expectedStatus string) {
	t.Helper()
	cfg := AppConfig{SQLiteDSN: sqliteDSN, RedisAddr: "skip", MQTTBroker: "skip"}
	app, err := NewApp(context.Background(), cfg)
	if err != nil {
		t.Fatalf("new app failed: %v", err)
	}

	wf := workflow.NewWorkflow(func(ctx context.Context, agentType string, nodeID string) error {
		return nil
	})
	if err := wf.AddTask(result.TaskID, "agent", nil); err != nil {
		t.Fatalf("add task failed: %v", err)
	}
	if err := wf.Start(context.Background()); err != nil {
		t.Fatalf("start workflow failed: %v", err)
	}

	app.mu.Lock()
	app.workflows[result.TaskID] = wf
	app.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- app.ListenResults(ctx)
	}()

	payload, _ := json.Marshal(result)
	if err := app.queue.Enqueue(context.Background(), "task_results", bus.Message{
		MessageID:  result.TaskID + ".result",
		SenderID:   result.TaskID,
		ReceiverID: "orchestrator",
		Payload:    payload,
		Timestamp:  time.Now().UnixMilli(),
	}); err != nil {
		t.Fatalf("enqueue result failed: %v", err)
	}

	var state memory.TaskState
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		state, err = app.taskStore.Get(context.Background(), result.TaskID)
		if err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("expected task state saved, got %v", err)
	}
	if state.Status != expectedStatus {
		t.Fatalf("expected %s status, got %s", expectedStatus, state.Status)
	}
	if result.Error != "" && state.ErrorMessage != result.Error {
		t.Fatalf("expected error message preserved, got %s", state.ErrorMessage)
	}

	nodeState, ok := wf.NodeState(result.TaskID)
	if !ok {
		t.Fatal("expected workflow node state")
	}
	if nodeState != workflow.NodeStateFailed {
		t.Fatalf("expected workflow node failed, got %s", nodeState)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil && err != context.Canceled {
			t.Fatalf("unexpected ListenResults error: %v", err)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("timed out waiting for ListenResults to exit")
	}
}

func TestGoldPathStoresSuccessResultAndCompletesWorkflow(t *testing.T) {
	cfg := AppConfig{SQLiteDSN: "file:orchestrator-success?mode=memory&cache=shared", RedisAddr: "skip", MQTTBroker: "skip"}
	app, err := NewApp(context.Background(), cfg)
	if err != nil {
		t.Fatalf("new app failed: %v", err)
	}

	task := bus.Message{
		MessageID:  "task-success",
		SenderID:   "user",
		ReceiverID: "orchestrator",
		Payload:    json.RawMessage(`{"task":"demo"}`),
		Timestamp:  time.Now().UnixMilli(),
	}
	if err := app.queue.Enqueue(context.Background(), "tasks", task); err != nil {
		t.Fatalf("enqueue task failed: %v", err)
	}
	app.proc = &fakeProcessManager{}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := app.ProcessOneTask(ctx); err != nil {
		t.Fatalf("ProcessOneTask failed: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- app.ListenResults(ctx)
	}()

	result := bus.TaskResult{
		TaskID:    "task-success",
		AgentName: "orchestrator",
		Status:    "success",
		Timestamp: time.Now().Unix(),
		Output:    json.RawMessage(`{"result":"ok"}`),
	}
	payload, _ := json.Marshal(result)
	if err := app.queue.Enqueue(context.Background(), "task_results", bus.Message{
		MessageID:  "task-success.result",
		SenderID:   "task-success",
		ReceiverID: "orchestrator",
		Payload:    payload,
		Timestamp:  time.Now().UnixMilli(),
	}); err != nil {
		t.Fatalf("enqueue result failed: %v", err)
	}

	var state memory.TaskState
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		state, err = app.taskStore.Get(context.Background(), "task-success")
		if err == nil && state.Status == "success" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("expected task state saved, got %v", err)
	}
	if state.Status != "success" {
		t.Fatalf("expected success status, got %s", state.Status)
	}

	app.mu.RLock()
	wf := app.workflows["task-success"]
	app.mu.RUnlock()
	if wf == nil {
		t.Fatal("expected workflow registered")
	}
	nodeState, ok := wf.NodeState("task-success")
	if !ok {
		t.Fatal("expected workflow node state")
	}
	if nodeState != workflow.NodeStateCompleted {
		t.Fatalf("expected workflow node completed, got %s", nodeState)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil && err != context.Canceled {
			t.Fatalf("unexpected ListenResults error: %v", err)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("timed out waiting for ListenResults to exit")
	}
}

func TestSingleTaskReplayAuditPreservesTerminalResultStatus(t *testing.T) {
	cfg := AppConfig{SQLiteDSN: "file:orchestrator-replay?mode=memory&cache=shared", RedisAddr: "skip", MQTTBroker: "skip"}
	app, err := NewApp(context.Background(), cfg)
	if err != nil {
		t.Fatalf("new app failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	auditEvents, err := app.events.Subscribe(ctx, "audit.task-replay")
	if err != nil {
		t.Fatalf("subscribe audit failed: %v", err)
	}

	task := bus.Message{
		MessageID:  "task-replay",
		SenderID:   "user",
		ReceiverID: "orchestrator",
		Payload:    json.RawMessage(`{"task":"demo replay"}`),
		Timestamp:  time.Now().UnixMilli(),
	}
	if err := app.queue.Enqueue(context.Background(), "tasks", task); err != nil {
		t.Fatalf("enqueue task failed: %v", err)
	}
	app.proc = &fakeProcessManager{}

	if err := app.ProcessOneTask(ctx); err != nil {
		t.Fatalf("ProcessOneTask failed: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- app.ListenResults(ctx)
	}()

	result := bus.TaskResult{
		TaskID:    "task-replay",
		AgentName: "planner",
		Status:    "timeout",
		Error:     "context deadline exceeded",
		Timestamp: time.Now().Unix(),
		Output:    json.RawMessage(`{"result":""}`),
	}
	payload, _ := json.Marshal(result)
	if err := app.queue.Enqueue(context.Background(), "task_results", bus.Message{
		MessageID:  "task-replay.result",
		SenderID:   "task-replay",
		ReceiverID: "orchestrator",
		Payload:    payload,
		Timestamp:  time.Now().UnixMilli(),
	}); err != nil {
		t.Fatalf("enqueue result failed: %v", err)
	}

	var events []llm.AuditEvent
	deadline := time.Now().Add(500 * time.Millisecond)
	for len(events) < 2 && time.Now().Before(deadline) {
		select {
		case msg := <-auditEvents:
			var event llm.AuditEvent
			if err := json.Unmarshal(msg.Payload, &event); err != nil {
				t.Fatalf("unmarshal audit event failed: %v", err)
			}
			events = append(events, event)
		case <-time.After(10 * time.Millisecond):
		}
	}

	if len(events) != 2 {
		t.Fatalf("expected 2 replay audit events, got %d: %+v", len(events), events)
	}
	if events[0].Event != "route" {
		t.Fatalf("expected first replay event route, got %s", events[0].Event)
	}
	if events[1].Event != "task_result" {
		t.Fatalf("expected second replay event task_result, got %s", events[1].Event)
	}
	if events[1].Status != "timeout" {
		t.Fatalf("expected terminal replay status timeout, got %s", events[1].Status)
	}
	if events[1].Error != "context deadline exceeded" {
		t.Fatalf("expected terminal replay error preserved, got %s", events[1].Error)
	}

	state, err := app.taskStore.Get(context.Background(), "task-replay")
	if err != nil {
		t.Fatalf("expected task state saved, got %v", err)
	}
	if state.Status != memory.TaskStatusTimeout {
		t.Fatalf("expected store status preserved as timeout, got %s", state.Status)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil && err != context.Canceled {
			t.Fatalf("unexpected ListenResults error: %v", err)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("timed out waiting for ListenResults to exit")
	}
}

type fakeProcessManager struct{}

func (f *fakeProcessManager) SpawnAgent(ctx context.Context, agentType string, taskID string) (int, error) {
	return 123, nil
}
func (f *fakeProcessManager) KillAgent(pid int) error {
	return nil
}

func TestProcessOneTaskTimesOutWhenQueueEmpty(t *testing.T) {
	cfg := AppConfig{SQLiteDSN: ":memory:", RedisAddr: "skip", MQTTBroker: "skip"}
	app, err := NewApp(context.Background(), cfg)
	if err != nil {
		t.Fatalf("new app failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err = app.ProcessOneTask(ctx)
	if err != nil && err != context.DeadlineExceeded {
		t.Fatalf("expected timeout error, got %v", err)
	}
}

func TestProcessLoopStopsOnContextCancel(t *testing.T) {
	cfg := AppConfig{SQLiteDSN: ":memory:", RedisAddr: "skip", MQTTBroker: "skip"}
	app, err := NewApp(context.Background(), cfg)
	if err != nil {
		t.Fatalf("new app failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := app.ProcessLoop(ctx); err != context.Canceled {
		t.Fatalf("expected context canceled, got %v", err)
	}
}

func TestParseAppConfigDefaultLoopDisabled(t *testing.T) {
	cfg, err := ParseAppConfig(nil)
	if err != nil {
		t.Fatalf("parse app config failed: %v", err)
	}
	if cfg.Loop {
		t.Fatal("expected loop mode disabled by default")
	}
}

func TestParseAppConfigAcceptsLoopFlag(t *testing.T) {
	cfg, err := ParseAppConfig([]string{"--loop"})
	if err != nil {
		t.Fatalf("parse app config failed: %v", err)
	}
	if !cfg.Loop {
		t.Fatal("expected loop mode enabled")
	}
}

func TestRunMainLoopReturnsCanceledOnCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := runMain(ctx, []string{"--sqlite-dsn", ":memory:", "--redis-addr", "skip", "--mqtt-broker", "skip", "--loop"}); err != context.Canceled {
		t.Fatalf("expected context canceled, got %v", err)
	}
}

func TestHandleApprovalPublishesUnifiedDecision(t *testing.T) {
	cfg := AppConfig{
		SQLiteDSN:      ":memory:",
		RedisAddr:      "skip",
		MQTTBroker:     "skip",
		ApprovalSecret: "test-secret",
	}
	app, err := NewApp(context.Background(), cfg)
	if err != nil {
		t.Fatalf("new app failed: %v", err)
	}

	approvalEvents, err := app.events.Subscribe(context.Background(), "approvals")
	if err != nil {
		t.Fatalf("subscribe approvals failed: %v", err)
	}

	decision := llm.ApprovalDecision{
		TraceID:    "trace-1",
		TaskID:     "task-1",
		ToolCallID: "call-1",
		Approved:   true,
		Reviewer:   "alice",
		Reason:     "approved",
	}
	body, _ := json.Marshal(decision)

	req := httptest.NewRequest(http.MethodPost, "/approval", strings.NewReader(string(body)))
	ts := time.Now().Unix()
	nonce := "nonce-1"
	req.Header.Set(skill.HeaderTimestamp, fmt.Sprintf("%d", ts))
	req.Header.Set(skill.HeaderNonce, nonce)
	req.Header.Set(skill.HeaderSignature, skill.GenerateSignature(cfg.ApprovalSecret, ts, nonce, body))

	rec := httptest.NewRecorder()
	app.handleApproval(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	select {
	case msg := <-approvalEvents:
		if msg.MessageID != "approval.task-1.call-1" {
			t.Fatalf("unexpected message id: %s", msg.MessageID)
		}
		var got llm.ApprovalDecision
		if err := json.Unmarshal(msg.Payload, &got); err != nil {
			t.Fatalf("unmarshal approval payload failed: %v", err)
		}
		if got.TaskID != "task-1" || got.ToolCallID != "call-1" {
			t.Fatalf("unexpected approval payload: %+v", got)
		}
		if got.DecidedAtMs <= 0 {
			t.Fatalf("expected decided_at_ms to be set, got %+v", got)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for approval event")
	}
}

func TestHandleApprovalPublishesApprovalAuditEvent(t *testing.T) {
	cfg := AppConfig{
		SQLiteDSN:      "file:approval-audit?mode=memory&cache=shared",
		RedisAddr:      "skip",
		MQTTBroker:     "skip",
		ApprovalSecret: "test-secret",
	}
	app, err := NewApp(context.Background(), cfg)
	if err != nil {
		t.Fatalf("new app failed: %v", err)
	}

	auditEvents, err := app.events.Subscribe(context.Background(), "audit.task-1")
	if err != nil {
		t.Fatalf("subscribe audit failed: %v", err)
	}

	decision := llm.ApprovalDecision{
		TraceID:    "trace-1",
		TaskID:     "task-1",
		ToolCallID: "call-1",
		Approved:   true,
		Reviewer:   "alice",
		Reason:     "approved",
	}
	body, _ := json.Marshal(decision)

	req := httptest.NewRequest(http.MethodPost, "/approval", strings.NewReader(string(body)))
	ts := time.Now().Unix()
	nonce := "nonce-audit"
	req.Header.Set(skill.HeaderTimestamp, fmt.Sprintf("%d", ts))
	req.Header.Set(skill.HeaderNonce, nonce)
	req.Header.Set(skill.HeaderSignature, skill.GenerateSignature(cfg.ApprovalSecret, ts, nonce, body))

	rec := httptest.NewRecorder()
	app.handleApproval(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	select {
	case msg := <-auditEvents:
		var event llm.AuditEvent
		if err := json.Unmarshal(msg.Payload, &event); err != nil {
			t.Fatalf("unmarshal audit event failed: %v", err)
		}
		if event.TaskID != "task-1" || event.Event != "approval_decision" {
			t.Fatalf("unexpected audit event: %+v", event)
		}
		if event.Actor != "orchestrator" {
			t.Fatalf("expected actor orchestrator, got %s", event.Actor)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("timed out waiting for approval audit event")
	}
}

func TestHandleApprovalRejectsInvalidSignature(t *testing.T) {
	cfg := AppConfig{
		SQLiteDSN:      ":memory:",
		RedisAddr:      "skip",
		MQTTBroker:     "skip",
		ApprovalSecret: "test-secret",
	}
	app, err := NewApp(context.Background(), cfg)
	if err != nil {
		t.Fatalf("new app failed: %v", err)
	}

	body := `{"task_id":"task-1","tool_call_id":"call-1","approved":true}`
	req := httptest.NewRequest(http.MethodPost, "/approval", strings.NewReader(body))
	req.Header.Set(skill.HeaderTimestamp, fmt.Sprintf("%d", time.Now().Unix()))
	req.Header.Set(skill.HeaderNonce, "nonce-invalid")
	req.Header.Set(skill.HeaderSignature, "bad-signature")

	rec := httptest.NewRecorder()
	app.handleApproval(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestServeHTTPCompletesWriteToolAfterApprovalWebhook(t *testing.T) {
	cfg := AppConfig{
		SQLiteDSN:      "file:orchestrator-webhook-approval-success?mode=memory&cache=shared",
		RedisAddr:      "skip",
		MQTTBroker:     "skip",
		ApprovalSecret: "test-secret",
	}
	app, err := NewApp(context.Background(), cfg)
	if err != nil {
		t.Fatalf("new app failed: %v", err)
	}

	provider := &orchestratorScriptedProvider{
		predictOutputs: []string{
			`{"think":"need approval","call_skill":{"id":"call-1","name":"write_test_note","arguments":{"note":"hello"},"is_write_operation":true}}`,
			`{"think":"done","final":"Write approved and executed."}`,
		},
	}
	app.resolver.Register("test-provider", provider)
	app.resolver.RegisterRoute(llm.StaticRoute{
		Alias:         "test-model",
		Provider:      "test-provider",
		UpstreamModel: "test-model-upstream",
	})
	registry := skill.NewRegistry()
	registry.Register(&orchestratorTestWriteSkill{})
	app.gatewayRegistry = registry

	chunks, err := app.events.Subscribe(context.Background(), "chunks.*")
	if err != nil {
		t.Fatalf("subscribe chunks failed: %v", err)
	}
	audits, err := app.events.Subscribe(context.Background(), "audit.*")
	if err != nil {
		t.Fatalf("subscribe audits failed: %v", err)
	}

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
		app.ServeHTTP(rec, req)
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
			approvalDecision := llm.ApprovalDecision{
				TraceID:     chunk.TraceID,
				TaskID:      chunk.TaskID,
				ToolCallID:  approvalReq.ToolCallID,
				Approved:    true,
				Reviewer:    "alice",
				Reason:      "approved",
				DecidedAtMs: time.Now().UnixMilli(),
			}
			approvalRec := httptest.NewRecorder()
			app.ServeHTTP(approvalRec, newSignedApprovalWebhookRequest(t, cfg.ApprovalSecret, approvalDecision))
			if approvalRec.Code != http.StatusOK {
				t.Fatalf("expected 200 approval webhook response, got %d", approvalRec.Code)
			}
		case <-deadline:
			t.Fatalf("timed out waiting for approval flow chunks, got %v", events)
		}
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for chat completion response")
	}

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	if payload["id"] != taskID {
		t.Fatalf("expected response id %s, got %+v", taskID, payload["id"])
	}
	choices, ok := payload["choices"].([]any)
	if !ok || len(choices) != 1 {
		t.Fatalf("unexpected choices payload: %+v", payload)
	}
	message, ok := choices[0].(map[string]any)["message"].(map[string]any)
	if !ok || message["content"] != "Write approved and executed." {
		t.Fatalf("unexpected final response payload: %+v", payload)
	}
	expected := []string{"tool_call", "waiting_approval", "tool_result", "final"}
	for idx, event := range expected {
		if events[idx] != event {
			t.Fatalf("expected events %v, got %v", expected, events)
		}
	}
	if provider.predictCalls != 2 {
		t.Fatalf("expected provider called twice, got %d", provider.predictCalls)
	}

	state, err := app.taskStore.Get(context.Background(), taskID)
	if err != nil {
		t.Fatalf("get task state failed: %v", err)
	}
	if state.Status != "success" {
		t.Fatalf("expected success task state, got %+v", state)
	}

	sawApprovalAudit := false
	auditDeadline := time.After(500 * time.Millisecond)
	for !sawApprovalAudit {
		select {
		case msg := <-audits:
			var event llm.AuditEvent
			if err := json.Unmarshal(msg.Payload, &event); err != nil {
				t.Fatalf("unmarshal audit event failed: %v", err)
			}
			if event.TaskID == taskID && event.Event == "approval_decision" && event.Status == "approved" {
				sawApprovalAudit = true
			}
		case <-auditDeadline:
			t.Fatalf("expected approval audit for task %s", taskID)
		}
	}
}

func TestServeHTTPLateApprovalWebhookDoesNotChangeTimedOutWriteToolState(t *testing.T) {
	cfg := AppConfig{
		SQLiteDSN:      "file:orchestrator-webhook-approval-late?mode=memory&cache=shared",
		RedisAddr:      "skip",
		MQTTBroker:     "skip",
		ApprovalSecret: "test-secret",
	}
	app, err := NewApp(context.Background(), cfg)
	if err != nil {
		t.Fatalf("new app failed: %v", err)
	}

	provider := &orchestratorScriptedProvider{
		predictOutputs: []string{
			`{"think":"need approval","call_skill":{"id":"call-1","name":"write_test_note","arguments":{"note":"hello"},"is_write_operation":true}}`,
		},
	}
	app.resolver.Register("test-provider", provider)
	app.resolver.RegisterRoute(llm.StaticRoute{
		Alias:         "test-model",
		Provider:      "test-provider",
		UpstreamModel: "test-model-upstream",
	})
	registry := skill.NewRegistry()
	registry.Register(&orchestratorTestWriteSkill{})
	app.gatewayRegistry = registry
	app.gatewayApprovalGate = &orchestratorTestApprovalGate{
		wait: func(ctx context.Context, req llm.ApprovalRequest, timeout time.Duration) (llm.ApprovalDecision, error) {
			time.Sleep(20 * time.Millisecond)
			return llm.ApprovalDecision{}, context.DeadlineExceeded
		},
	}

	chunks, err := app.events.Subscribe(context.Background(), "chunks.*")
	if err != nil {
		t.Fatalf("subscribe chunks failed: %v", err)
	}
	audits, err := app.events.Subscribe(context.Background(), "audit.*")
	if err != nil {
		t.Fatalf("subscribe audits failed: %v", err)
	}

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
		app.ServeHTTP(rec, req)
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
		t.Fatal("timed out waiting for timed-out response")
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

	state, err := app.taskStore.Get(context.Background(), taskID)
	if err != nil {
		t.Fatalf("get task state failed: %v", err)
	}
	if state.Status != "timeout" {
		t.Fatalf("expected timeout task state, got %+v", state)
	}

	lateDecision := llm.ApprovalDecision{
		TraceID:     taskID,
		TaskID:      taskID,
		ToolCallID:  "call-1",
		Approved:    true,
		Reviewer:    "alice",
		Reason:      "late approval",
		DecidedAtMs: time.Now().UnixMilli(),
	}
	approvalRec := httptest.NewRecorder()
	app.ServeHTTP(approvalRec, newSignedApprovalWebhookRequest(t, cfg.ApprovalSecret, lateDecision))
	if approvalRec.Code != http.StatusOK {
		t.Fatalf("expected 200 approval webhook response, got %d", approvalRec.Code)
	}

	stateAfterLateDecision, err := app.taskStore.Get(context.Background(), taskID)
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

	sawApprovalAudit := false
	auditDeadline := time.After(500 * time.Millisecond)
	for !sawApprovalAudit {
		select {
		case msg := <-audits:
			var event llm.AuditEvent
			if err := json.Unmarshal(msg.Payload, &event); err != nil {
				t.Fatalf("unmarshal audit event failed: %v", err)
			}
			if event.TaskID == taskID && event.Event == "approval_decision" && event.Status == "approved" {
				sawApprovalAudit = true
			}
		case <-auditDeadline:
			t.Fatalf("expected late approval audit for task %s", taskID)
		}
	}
}
