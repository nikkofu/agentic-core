package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"sync"
	"time"

	"agentic-core/internal/bus"
	"agentic-core/internal/gateway"
	"agentic-core/internal/llm"
	"agentic-core/internal/logging"
	"agentic-core/internal/memory"
	"agentic-core/internal/process"
	"agentic-core/internal/skill"
	"agentic-core/internal/workflow"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/redis/go-redis/v9"
	_ "modernc.org/sqlite"
)

type AppConfig struct {
	SQLiteDSN      string
	RedisAddr      string
	MQTTBroker     string
	SubagentBinary string
	HTTPPort       string
	Loop           bool
	LLMAPIKey      string
	LLMBaseURL     string
	ApprovalSecret string
}

type App struct {
	cfg        AppConfig
	db         *sql.DB
	taskStore  memory.TaskStateStore
	queue      bus.TaskQueue
	events     bus.EventBus
	heartbeat  bus.HeartbeatBus
	proc       process.ProcessManager
	registry   *process.AgentRegistry
	resolver   *llm.ModelResolver
	sender     *gateway.Sender
	nonceStore skill.NonceStore
	auditor    *process.Auditor
	logger     *slog.Logger

	gatewayRegistry     *skill.Registry
	gatewayApprovalGate llm.ApprovalGate
	chatHandler         http.Handler

	mu        sync.RWMutex
	workflows map[string]*workflow.Workflow
}

func ParseAppConfig(args []string) (AppConfig, error) {
	fs := flag.NewFlagSet("orchestrator", flag.ContinueOnError)
	cfg := AppConfig{}
	fs.StringVar(&cfg.SQLiteDSN, "sqlite-dsn", "agentic_core.db", "sqlite dsn")
	fs.StringVar(&cfg.RedisAddr, "redis-addr", "localhost:16379", "redis address (set to 'skip' for tests)")
	fs.StringVar(&cfg.MQTTBroker, "mqtt-broker", "tcp://localhost:11883", "mqtt broker address (set to 'skip' for tests)")
	fs.StringVar(&cfg.SubagentBinary, "subagent-binary", "./subagent", "path to subagent binary")
	fs.StringVar(&cfg.HTTPPort, "http-port", ":8080", "http port")
	fs.BoolVar(&cfg.Loop, "loop", false, "run continuous task processing loop")
	fs.StringVar(&cfg.LLMAPIKey, "llm-api-key", os.Getenv("LLM_API_KEY"), "llm api key")
	fs.StringVar(&cfg.LLMBaseURL, "llm-base-url", os.Getenv("LLM_BASE_URL"), "llm base url")
	fs.StringVar(&cfg.ApprovalSecret, "approval-secret", os.Getenv("APPROVAL_WEBHOOK_SECRET"), "shared secret for approval webhook signature verification")
	if err := fs.Parse(args); err != nil {
		return AppConfig{}, err
	}
	return cfg, nil
}

func NewApp(ctx context.Context, cfg AppConfig) (*App, error) {
	db, err := sql.Open("sqlite", cfg.SQLiteDSN)
	if err != nil {
		return nil, err
	}

	store := memory.NewSQLiteTaskStateStore(db)
	if err := store.InitSchema(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}

	app := &App{
		cfg:        cfg,
		db:         db,
		taskStore:  store,
		registry:   process.NewAgentRegistry(),
		workflows:  make(map[string]*workflow.Workflow),
		resolver:   llm.NewModelResolver(),
		nonceStore: skill.NewInMemNonceStore(),
	}

	// Initialize Redis unless skipped
	if cfg.RedisAddr != "skip" {
		rdb := redis.NewClient(&redis.Options{
			Addr: cfg.RedisAddr,
		})
		if err := rdb.Ping(ctx).Err(); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("redis connection failed: %w", err)
		}
		transport := bus.NewRedisTransport(rdb)
		app.queue = transport
		app.events = transport
	} else {
		transport := bus.NewFakeTransport()
		app.queue = transport
		app.events = transport
	}

	app.sender = gateway.NewSender(app.events)
	_ = app.sender.Start(ctx)
	app.logger = logging.Component("orchestrator")
	app.auditor = process.NewAuditor(app.events, "orchestrator")
	app.gatewayApprovalGate = skill.NewApprovalGate(app.events)

	// 初始化默认模型路由
	if cfg.LLMAPIKey != "" {
		provider := llm.NewOpenAIProvider(llm.ModelConfig{
			ProviderName: "openai",
			ModelID:      "gpt-4",
			APIKey:       cfg.LLMAPIKey,
			BaseURL:      cfg.LLMBaseURL,
		})
		app.resolver.Register("openai", provider)
		app.resolver.RegisterRoute(llm.StaticRoute{
			Alias:         "gpt-4",
			Provider:      "openai",
			UpstreamModel: "gpt-4",
		})
	}

	// Initialize MQTT unless skipped
	if cfg.MQTTBroker != "skip" {
		opts := mqtt.NewClientOptions().AddBroker(cfg.MQTTBroker).SetClientID("orchestrator")
		mclient := mqtt.NewClient(opts)
		if token := mclient.Connect(); token.Wait() && token.Error() != nil {
			_ = db.Close()
			return nil, fmt.Errorf("mqtt connection failed: %w", token.Error())
		}
		app.heartbeat = bus.NewMQTTHeartbeatBus(mclient)
	} else {
		app.heartbeat = bus.NewFakeHeartbeatBus()
	}

	// 初始化进程管理器
	app.proc = &process.ExecProcessManager{
		BinaryPath: cfg.SubagentBinary,
	}

	return app, nil
}

func (a *App) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	mux := http.NewServeMux()
	mux.HandleFunc("/approval", a.handleApproval)
	mux.Handle("/v1/chat/completions", a.chatCompletionsHandler())
	mux.ServeHTTP(w, r)
}

func (a *App) chatCompletionsHandler() http.Handler {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.chatHandler == nil {
		a.chatHandler = gateway.NewChatCompletionsHandlerWithStoreRegistryAndApprovalGate(
			a.resolver,
			a.sender,
			a.taskStore,
			a.gatewayRegistry,
			a.gatewayApprovalGate,
		)
	}
	return a.chatHandler
}

func (a *App) handleApproval(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Invalid body", http.StatusBadRequest)
		return
	}

	if a.cfg.ApprovalSecret != "" {
		if err := skill.VerifyWebhookSignature(r.Header, body, a.cfg.ApprovalSecret, a.nonceStore, time.Now()); err != nil {
			http.Error(w, "Invalid signature", http.StatusUnauthorized)
			return
		}
	}

	var decision llm.ApprovalDecision
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&decision); err != nil {
		http.Error(w, "Invalid body", http.StatusBadRequest)
		return
	}
	if decision.DecidedAtMs == 0 {
		decision.DecidedAtMs = time.Now().UnixMilli()
	}
	if err := decision.Validate(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	payload, _ := json.Marshal(decision)
	msg := bus.Message{
		MessageID:  fmt.Sprintf("approval.%s.%s", decision.TaskID, decision.ToolCallID),
		SenderID:   "orchestrator",
		ReceiverID: decision.TaskID,
		Payload:    payload,
		Timestamp:  time.Now().UnixMilli(),
	}

	_ = a.events.Publish(r.Context(), "approvals", msg)
	_ = a.auditor.Record(r.Context(), llm.AuditEvent{
		TraceID:     decision.TraceID,
		TaskID:      decision.TaskID,
		ToolCallID:  decision.ToolCallID,
		Event:       "approval_decision",
		Actor:       "orchestrator",
		Status:      map[bool]string{true: "approved", false: "rejected"}[decision.Approved],
		Data:        payload,
		TimestampMs: decision.DecidedAtMs,
	})

	a.logger.Info("approval decision received",
		"task_id", decision.TaskID,
		"tool_call_id", decision.ToolCallID,
		"approved", decision.Approved,
	)
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func (a *App) Run(ctx context.Context) error {
	msg := bus.Message{
		MessageID:  "orchestrator.health",
		SenderID:   "orchestrator",
		ReceiverID: "orchestrator",
		Payload:    json.RawMessage(`{"status":"running"}`),
		Timestamp:  time.Now().UnixMilli(),
	}
	_ = a.events.Publish(ctx, "system.health", msg)
	_ = a.heartbeat.PublishHeartbeat(ctx, "orchestrator", "running")

	select {
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (a *App) OnNodeReady(ctx context.Context, agentType string, nodeID string) error {
	extraArgs := []string{
		"--redis-addr", a.cfg.RedisAddr,
		"--mqtt-broker", a.cfg.MQTTBroker,
	}

	// Get agent profile if exists
	profile, err := a.registry.GetProfile(agentType)
	if err == nil {
		a.logger.Info("spawning agent profile", "agent_type", agentType, "profile_name", profile.Name, "description", profile.Description)
	}

	pid, err := a.proc.SpawnAgent(ctx, agentType, nodeID, extraArgs...)
	if err != nil {
		return err
	}
	a.logger.Info("workflow node spawned", "node_id", nodeID, "agent_type", agentType, "pid", pid)
	return nil
}

func (a *App) ListenResults(ctx context.Context) error {
	results, err := a.queue.Dequeue(ctx, "task_results")
	if err != nil {
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg, ok := <-results:
			if !ok {
				return nil
			}
			var res bus.TaskResult
			_ = json.Unmarshal(msg.Payload, &res)

			state := memory.TaskState{
				TaskID:        res.TaskID,
				ParentTaskID:  res.ParentTaskID,
				AgentName:     res.AgentName,
				Status:        normalizeTaskStatus(res.Status),
				UpdatedAtUnix: time.Now().Unix(),
				ErrorMessage:  res.Error,
			}
			_ = a.taskStore.Save(ctx, state)

			a.mu.RLock()
			wf, exists := a.workflows[res.TaskID]
			a.mu.RUnlock()
			if exists {
				if res.Status == "success" {
					_ = wf.MarkCompleted(ctx, res.TaskID)
				} else {
					_ = wf.MarkFailed(ctx, res.TaskID, res.Error)
				}
			}

			timestampMs := msg.Timestamp
			if timestampMs == 0 {
				timestampMs = time.Now().UnixMilli()
			}
			actor := msg.SenderID
			if actor == "" {
				actor = res.AgentName
			}
			_ = a.auditor.Record(ctx, llm.AuditEvent{
				TaskID:       res.TaskID,
				ParentTaskID: res.ParentTaskID,
				Event:        "task_result",
				Actor:        actor,
				Status:       res.Status,
				Error:        res.Error,
				Data:         msg.Payload,
				TimestampMs:  timestampMs,
			})

			if res.ParentTaskID != "" {
				a.logger.Info("subtask finished", "task_id", res.TaskID, "parent_task_id", res.ParentTaskID, "agent_name", res.AgentName, "status", res.Status)
			}
		}
	}
}

func (a *App) ProcessOneTask(ctx context.Context) error {
	msgChan, err := a.queue.Dequeue(ctx, "tasks")
	if err != nil {
		return err
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case msg, ok := <-msgChan:
		if !ok {
			return errors.New("tasks channel closed")
		}
		agentType := msg.TargetAgent
		if agentType == "" {
			agentType = "orchestrator" // Default
		}

		wf := workflow.NewWorkflow(a.OnNodeReady)
		_ = wf.AddTask(msg.MessageID, agentType, nil)
		a.mu.Lock()
		a.workflows[msg.MessageID] = wf
		a.mu.Unlock()

		taskChannel := "task." + msg.MessageID
		_ = a.queue.Enqueue(ctx, taskChannel, msg)
		if err := wf.Start(ctx); err != nil {
			_ = a.taskStore.Save(ctx, memory.TaskState{
				TaskID:        msg.MessageID,
				ParentTaskID:  msg.ParentTaskID,
				AgentName:     agentType,
				Status:        "failed",
				UpdatedAtUnix: time.Now().Unix(),
				ErrorMessage:  err.Error(),
			})
			return err
		}

		if err := a.taskStore.Save(ctx, memory.TaskState{
			TaskID:        msg.MessageID,
			ParentTaskID:  msg.ParentTaskID,
			AgentName:     agentType,
			Status:        "running",
			UpdatedAtUnix: time.Now().Unix(),
		}); err != nil {
			return err
		}

		_ = a.auditor.Record(ctx, llm.AuditEvent{
			TaskID:       msg.MessageID,
			ParentTaskID: msg.ParentTaskID,
			Event:        "route",
			Actor:        "orchestrator",
			Status:       "running",
			Data:         msg.Payload,
			TimestampMs:  time.Now().UnixMilli(),
		})

		return nil
	}
}

func normalizeTaskStatus(status string) string {
	switch status {
	case "pending", "running", "success", "failed":
		return status
	default:
		return "failed"
	}
}

func (a *App) ProcessLoop(ctx context.Context) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := a.ProcessOneTask(ctx)
		if err == nil {
			continue
		}
		if errors.Is(err, bus.ErrNoMessage) {
			time.Sleep(10 * time.Millisecond)
			continue
		}
		return err
	}
}

func runMain(ctx context.Context, args []string) error {
	cfg, err := ParseAppConfig(args)
	if err != nil {
		return err
	}
	if _, err := logging.Init(logging.DefaultConfig("orchestrator")); err != nil {
		return err
	}
	app, err := NewApp(ctx, cfg)
	if err != nil {
		return err
	}
	defer app.db.Close()

	go func() {
		if err := app.ListenResults(ctx); err != nil && !errors.Is(err, context.Canceled) {
			logging.Component("orchestrator").Error("listen results failed", "error", err.Error())
		}
	}()

	server := &http.Server{Addr: cfg.HTTPPort, Handler: app}
	go func() {
		logging.Component("orchestrator").Info("webhook server listening", "addr", cfg.HTTPPort)
		_ = server.ListenAndServe()
	}()
	defer server.Shutdown(context.Background())

	if cfg.Loop {
		return app.ProcessLoop(ctx)
	}
	return app.Run(ctx)
}

func main() {
	if err := runMain(context.Background(), os.Args[1:]); err != nil {
		os.Exit(1)
	}
}
