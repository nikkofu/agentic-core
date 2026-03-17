package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"agentic-core/internal/bus"
	"agentic-core/internal/memory"
	"agentic-core/internal/process"
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
}

type App struct {
	cfg       AppConfig
	db        *sql.DB
	taskStore memory.TaskStateStore
	pubsub    bus.PubSub
	heartbeat bus.HeartbeatBus
	proc      process.ProcessManager
	registry  *process.AgentRegistry

	mu        sync.RWMutex
	workflows map[string]*workflow.Workflow
}

func ParseAppConfig(args []string) (AppConfig, error) {
	fs := flag.NewFlagSet("orchestrator", flag.ContinueOnError)
	cfg := AppConfig{}
	fs.StringVar(&cfg.SQLiteDSN, "sqlite-dsn", "agentic_core.db", "sqlite dsn")
	fs.StringVar(&cfg.RedisAddr, "redis-addr", "localhost:6379", "redis address (set to 'skip' for tests)")
	fs.StringVar(&cfg.MQTTBroker, "mqtt-broker", "tcp://localhost:1883", "mqtt broker address (set to 'skip' for tests)")
	fs.StringVar(&cfg.SubagentBinary, "subagent-binary", "./subagent", "path to subagent binary")
	fs.StringVar(&cfg.HTTPPort, "http-port", ":8080", "http webhook port")
	fs.BoolVar(&cfg.Loop, "loop", false, "run continuous task processing loop")
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
		cfg:       cfg,
		db:        db,
		taskStore: store,
		registry:  process.NewAgentRegistry(),
		workflows: make(map[string]*workflow.Workflow),
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
		app.pubsub = bus.NewRedisPubSub(rdb)
	} else {
		app.pubsub = bus.NewFakePubSub()
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
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var resp bus.ApprovalResponse
	if err := json.NewDecoder(r.Body).Decode(&resp); err != nil {
		http.Error(w, "Invalid body", http.StatusBadRequest)
		return
	}

	payload, _ := json.Marshal(resp)
	msg := bus.Message{
		MessageID:  "approval." + resp.TaskID,
		SenderID:   "orchestrator",
		ReceiverID: resp.TaskID,
		Payload:    payload,
		Timestamp:  time.Now().UnixMilli(),
	}

	if rps, ok := a.pubsub.(*bus.RedisPubSub); ok {
		_ = rps.Broadcast(r.Context(), "approvals", msg)
	} else {
		_ = a.pubsub.Publish(r.Context(), "approvals", msg)
	}

	fmt.Printf("HITL: Task %s approval received: %v\n", resp.TaskID, resp.Approved)
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
	_ = a.pubsub.Publish(ctx, "system.health", msg)
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
		fmt.Printf("Spawning Agent: %s (%s)\n", profile.Name, profile.Description)
	}

	pid, err := a.proc.SpawnAgent(ctx, agentType, nodeID, extraArgs...)
	if err != nil {
		return err
	}
	fmt.Printf("Workflow node %s (type: %s) spawned as PID %d\n", nodeID, agentType, pid)
	return nil
}

func (a *App) ListenResults(ctx context.Context) error {
	rps, ok := a.pubsub.(*bus.RedisPubSub)
	if !ok {
		return errors.New("pubsub is not RedisPubSub")
	}

	results := rps.Subscribe(ctx, "task_results")
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
				Status:        res.Status,
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
			
			if res.ParentTaskID != "" {
				fmt.Printf("Subtask %s of %s (Agent: %s) finished with status %s\n", res.TaskID, res.ParentTaskID, res.AgentName, res.Status)
			}
		}
	}
}

func (a *App) ProcessOneTask(ctx context.Context) error {
	msg, err := a.pubsub.Consume(ctx, "tasks")
	if err != nil {
		return err
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
	_ = a.pubsub.Publish(ctx, taskChannel, msg)
	return wf.Start(ctx)
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
	app, err := NewApp(ctx, cfg)
	if err != nil {
		return err
	}
	defer app.db.Close()

	go func() {
		if err := app.ListenResults(ctx); err != nil && !errors.Is(err, context.Canceled) {
			fmt.Fprintf(os.Stderr, "ListenResults error: %v\n", err)
		}
	}()

	server := &http.Server{Addr: cfg.HTTPPort, Handler: app}
	go func() {
		fmt.Printf("HITL Webhook Server listening on %s\n", cfg.HTTPPort)
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
