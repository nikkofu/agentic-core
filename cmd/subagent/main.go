package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"agentic-core/internal/bus"
	"agentic-core/internal/llm"
	"agentic-core/internal/logging"
	"agentic-core/internal/memory"
	"agentic-core/internal/process"
	"agentic-core/internal/runtimepaths"
	"agentic-core/internal/session"
	"agentic-core/internal/skill"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/redis/go-redis/v9"
	_ "modernc.org/sqlite"
)

type Config struct {
	AgentType  string
	TaskID     string
	RedisAddr  string
	MQTTBroker string
	SQLiteDSN  string

	// LLM Config
	LLMProvider string
	LLMModel    string
	LLMAPIKey   string
	LLMBaseURL  string
}

type Subagent struct {
	cfg       Config
	queue     bus.TaskQueue
	events    bus.EventBus
	heartbeat bus.HeartbeatBus
	runtime   runtimeExecutor
	history   session.HistoryStore
	resolver  *llm.ModelResolver
	auditor   *process.Auditor
	logger    *slog.Logger
}

type runtimeExecutor interface {
	Run(ctx context.Context, req llm.InferenceRequest, fanout *llm.Fanout) (llm.FinalResult, error)
}

func ParseConfig(args []string) (Config, error) {
	fs := flag.NewFlagSet("subagent", flag.ContinueOnError)
	var cfg Config
	fs.StringVar(&cfg.AgentType, "agent-type", "", "subagent type")
	fs.StringVar(&cfg.TaskID, "task-id", "", "task id")
	fs.StringVar(&cfg.RedisAddr, "redis-addr", "localhost:16379", "redis address")
	fs.StringVar(&cfg.MQTTBroker, "mqtt-broker", "tcp://localhost:11883", "mqtt broker address")
	fs.StringVar(&cfg.SQLiteDSN, "sqlite-dsn", filepath.Join("var", "db", "agentic_core.db"), "sqlite dsn")

	// LLM Flags
	fs.StringVar(&cfg.LLMProvider, "llm-provider", "openai", "llm provider (openai, etc)")
	fs.StringVar(&cfg.LLMModel, "llm-model", "gpt-4", "llm model id")
	fs.StringVar(&cfg.LLMAPIKey, "llm-api-key", os.Getenv("LLM_API_KEY"), "llm api key")
	fs.StringVar(&cfg.LLMBaseURL, "llm-base-url", os.Getenv("LLM_BASE_URL"), "llm base url")

	if err := fs.Parse(args); err != nil {
		return Config{}, err
	}
	if cfg.TaskID == "" {
		return Config{}, fmt.Errorf("task-id is required")
	}
	return cfg, nil
}

func resolveSQLiteDSNForOpen(dsn string, cwd string) (string, error) {
	if dsn == "" || dsn == ":memory:" {
		return dsn, nil
	}
	if strings.HasPrefix(strings.ToLower(dsn), "file:") {
		return dsn, nil
	}
	if filepath.IsAbs(dsn) {
		return dsn, nil
	}
	if dsn != filepath.Join("var", "db", "agentic_core.db") {
		return dsn, nil
	}

	runtimeRoot, err := runtimepaths.ResolveRuntimeRoot("", cwd)
	if err != nil {
		return "", err
	}
	return filepath.Join(runtimeRoot, dsn), nil
}

func prepareSQLiteDSNDir(dsn string) error {
	parentDir, needsMkdir := runtimepaths.SQLiteDSNParentDirToPrepare(dsn)
	if !needsMkdir {
		return nil
	}
	if parentDir == "" {
		return nil
	}
	if err := os.MkdirAll(parentDir, 0o755); err != nil {
		return fmt.Errorf("create sqlite parent dir %q: %w", parentDir, err)
	}
	return nil
}

func NewSubagent(ctx context.Context, cfg Config) (*Subagent, error) {
	s := &Subagent{cfg: cfg}

	// 初始化 SQLite 会话存储
	if cfg.SQLiteDSN != "skip" {
		sqliteDSN, err := resolveSQLiteDSNForOpen(cfg.SQLiteDSN, ".")
		if err != nil {
			return nil, err
		}
		if err := prepareSQLiteDSNDir(sqliteDSN); err != nil {
			return nil, err
		}
		db, err := sql.Open("sqlite", sqliteDSN)
		if err == nil {
			historyStore := session.NewSQLiteHistoryStore(db)
			_ = historyStore.InitSchema(ctx)
			s.history = historyStore
			cfg.SQLiteDSN = sqliteDSN
			s.cfg.SQLiteDSN = sqliteDSN
		}
	}

	// 初始化 LLM Resolver
	s.resolver = llm.NewModelResolver()
	if cfg.LLMAPIKey != "" {
		provider := llm.NewOpenAIProvider(llm.ModelConfig{
			ProviderName: cfg.LLMProvider,
			ModelID:      cfg.LLMModel,
			APIKey:       cfg.LLMAPIKey,
			BaseURL:      cfg.LLMBaseURL,
			Temperature:  0.1,
		})
		s.resolver.Register(cfg.LLMProvider, provider)
	} else if cfg.RedisAddr == "skip" {
		// 测试模式：允许无 API Key
	} else {
		return nil, fmt.Errorf("LLM_API_KEY is required")
	}

	// Initialize Redis unless skipped
	if cfg.RedisAddr != "skip" {
		rdb := redis.NewClient(&redis.Options{
			Addr: cfg.RedisAddr,
		})
		if err := rdb.Ping(ctx).Err(); err != nil {
			return nil, fmt.Errorf("redis connection failed: %w", err)
		}
		transport := bus.NewRedisTransport(rdb)
		s.queue = transport
		s.events = transport
	} else {
		transport := bus.NewFakeTransport()
		s.queue = transport
		s.events = transport
	}
	s.logger = logging.Component("subagent")
	s.auditor = process.NewAuditor(s.events, s.cfg.TaskID)

	// Initialize MQTT unless skipped
	if cfg.MQTTBroker != "skip" {
		opts := mqtt.NewClientOptions().AddBroker(cfg.MQTTBroker).SetClientID(fmt.Sprintf("subagent-%s-%d", cfg.AgentType, time.Now().UnixNano()))
		mclient := mqtt.NewClient(opts)
		if token := mclient.Connect(); token.Wait() && token.Error() != nil {
			return nil, fmt.Errorf("mqtt connection failed: %w", token.Error())
		}
		s.heartbeat = bus.NewMQTTHeartbeatBus(mclient)
	} else {
		s.heartbeat = bus.NewFakeHeartbeatBus()
	}

	// 初始化 Runtime
	registry := skill.NewRegistry()
	registry.Register(&skill.CurrentTimeSkill{})
	registry.Register(&skill.HttpGetSkill{})

	executor := skill.NewExecutor(registry)
	gate := skill.NewApprovalGate(s.events)

	p, _ := s.resolver.Resolve(cfg.LLMProvider)
	s.runtime = llm.NewRuntime(p, executor, gate)

	return s, nil
}

func (s *Subagent) startHeartbeat(ctx context.Context) {
	_ = s.heartbeat.PublishHeartbeat(ctx, s.cfg.TaskID, "running")
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = s.heartbeat.PublishHeartbeat(ctx, s.cfg.TaskID, "running")
		}
	}
}

func (s *Subagent) Run(ctx context.Context) error {
	// 启动心跳
	go s.startHeartbeat(ctx)

	// 监听任务频道
	taskChannel := "task." + s.cfg.TaskID
	msgChan, err := s.queue.Dequeue(ctx, taskChannel)
	if err != nil {
		return fmt.Errorf("failed to consume task: %w", err)
	}

	for msg := range msgChan {
		s.logger.Info("executing task", "agent_type", s.cfg.AgentType, "task_id", s.cfg.TaskID, "payload", string(msg.Payload))

		// 尝试从 Payload 解析 SessionID 和 Task
		var payloadMap map[string]interface{}
		var sessionID string
		var actualTask string

		if err := json.Unmarshal(msg.Payload, &payloadMap); err == nil {
			if sid, ok := payloadMap["session_id"].(string); ok {
				sessionID = sid
			}
			if taskText, ok := payloadMap["task"].(string); ok {
				actualTask = taskText
			} else {
				actualTask = string(msg.Payload)
			}
		} else {
			actualTask = string(msg.Payload)
		}

		// 获取历史记录
		var history []session.ChatMessage
		if s.history != nil && sessionID != "" {
			h, _ := s.history.GetHistory(ctx, sessionID, 10)
			history = h
		}

		// 构造系统提示词
		builder := llm.NewSystemPromptBuilder(s.cfg.AgentType)
		builder.AddInstruction("Analyze the user request and use tools if needed.")
		builder.AddInstruction("Your response MUST be a valid JSON object matching the requested schema.")
		systemPrompt := builder.Build()

		// 使用 PromptBuilder 组装消息 (带自动压缩)
		provider, _ := s.resolver.Get(s.cfg.LLMProvider)
		pb := llm.NewPromptBuilder(4096)
		messages := pb.BuildMessages(ctx, provider, systemPrompt, history, actualTask)

		traceID := fmt.Sprintf("tr_%d", time.Now().UnixNano())
		fanout := llm.NewFanout(traceID, sessionID, s.cfg.TaskID)
		fanout.SetPublisher(s.publishChunk)

		infReq := llm.InferenceRequest{
			TraceID:    traceID,
			SessionID:  sessionID,
			TaskID:     s.cfg.TaskID,
			Messages:   messages,
			ModelAlias: s.cfg.LLMProvider,
		}

		// 执行运行时循环
		result, err := s.runtime.Run(ctx, infReq, fanout)
		if err != nil {
			process.LogAction(s.cfg.TaskID, "run", "ERROR", map[string]string{"error": err.Error()})
			// 发送错误结果
			s.sendResult(msg, runtimeTaskResultStatus(result.Status, err), "", err.Error())
			continue
		}

		// 保存到历史记录
		if s.history != nil && sessionID != "" {
			_ = s.history.AddMessage(ctx, session.ChatMessage{
				ID:        fmt.Sprintf("msg_%d_user", time.Now().UnixNano()),
				SessionID: sessionID,
				Role:      "user",
				Content:   actualTask,
			})
			_ = s.history.AddMessage(ctx, session.ChatMessage{
				ID:        fmt.Sprintf("msg_%d_assistant", time.Now().UnixNano()),
				SessionID: sessionID,
				Role:      "assistant",
				Content:   result.Content,
			})
		}

		// 发送成功结果
		s.sendResult(msg, memory.TaskStatusSuccess, result.Content, "")
	}

	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

func (s *Subagent) publishChunk(ctx context.Context, chunk llm.StreamChunk) error {
	if chunk.TaskID == "" {
		chunk.TaskID = s.cfg.TaskID
	}

	payload, err := json.Marshal(chunk)
	if err != nil {
		return err
	}

	if err := s.events.Publish(ctx, "chunks."+chunk.TaskID, bus.Message{
		MessageID:  fmt.Sprintf("chunk.%s.%d", chunk.TaskID, chunk.Sequence),
		SenderID:   s.cfg.TaskID,
		ReceiverID: "sender",
		Payload:    payload,
		Timestamp:  time.Now().UnixMilli(),
	}); err != nil {
		return err
	}

	return s.auditor.Record(ctx, llm.AuditEvent{
		TraceID:     chunk.TraceID,
		SessionID:   chunk.SessionID,
		TaskID:      chunk.TaskID,
		Event:       chunk.Event,
		Actor:       s.cfg.TaskID,
		Error:       chunk.Error,
		Data:        chunk.Data,
		TimestampMs: chunk.TimestampMs,
	})
}

func (s *Subagent) sendResult(msg bus.Message, status, output, err string) {
	result := bus.TaskResult{
		TaskID:       msg.MessageID,
		ParentTaskID: msg.ParentTaskID,
		AgentName:    s.cfg.AgentType,
		Status:       status,
		Output:       json.RawMessage(`{"result":"` + output + `"}`),
		Error:        err,
		Timestamp:    time.Now().Unix(),
	}
	resPayload, _ := json.Marshal(result)
	_ = s.queue.Enqueue(context.Background(), "task_results", bus.Message{
		MessageID:  msg.MessageID + ".result",
		SenderID:   s.cfg.TaskID,
		ReceiverID: "orchestrator",
		Payload:    resPayload,
		Timestamp:  time.Now().UnixMilli(),
	})
}

func runtimeTaskResultStatus(runtimeStatus string, err error) string {
	normalized := memory.NormalizeTaskStatus(runtimeStatus)
	if runtimeStatus != "" {
		return normalized
	}
	if err != nil {
		return memory.TaskStatusFailed
	}
	return memory.TaskStatusSuccess
}

func main() {
	cfg, err := ParseConfig(os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing config: %v\n", err)
		os.Exit(1)
	}
	if _, err := logging.Init(logging.DefaultConfig("subagent")); err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing logging: %v\n", err)
		os.Exit(1)
	}

	ctx := context.Background()
	s, err := NewSubagent(ctx, cfg)
	if err != nil {
		logging.Component("subagent").Error("failed to create subagent", "error", err.Error())
		os.Exit(1)
	}

	if err := s.Run(ctx); err != nil {
		logging.Component("subagent").Error("subagent run failed", "error", err.Error())
		os.Exit(1)
	}
}
