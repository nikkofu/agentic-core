package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"agentic-core/internal/bus"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/redis/go-redis/v9"
)

type Config struct {
	AgentType  string
	TaskID     string
	RedisAddr  string
	MQTTBroker string
}

type Subagent struct {
	cfg       Config
	pubsub    bus.PubSub
	heartbeat bus.HeartbeatBus
}

func ParseConfig(args []string) (Config, error) {
	fs := flag.NewFlagSet("subagent", flag.ContinueOnError)
	var cfg Config
	fs.StringVar(&cfg.AgentType, "agent-type", "", "subagent type")
	fs.StringVar(&cfg.TaskID, "task-id", "", "task id")
	fs.StringVar(&cfg.RedisAddr, "redis-addr", "localhost:6379", "redis address")
	fs.StringVar(&cfg.MQTTBroker, "mqtt-broker", "tcp://localhost:1883", "mqtt broker address")

	if err := fs.Parse(args); err != nil {
		return Config{}, err
	}
	if cfg.AgentType == "" || cfg.TaskID == "" {
		return Config{}, errors.New("agent-type and task-id are required")
	}
	return cfg, nil
}

func NewSubagent(ctx context.Context, cfg Config) (*Subagent, error) {
	s := &Subagent{cfg: cfg}

	// Initialize Redis unless skipped
	if cfg.RedisAddr != "skip" {
		rdb := redis.NewClient(&redis.Options{
			Addr: cfg.RedisAddr,
		})
		if err := rdb.Ping(ctx).Err(); err != nil {
			return nil, fmt.Errorf("redis connection failed: %w", err)
		}
		s.pubsub = bus.NewRedisPubSub(rdb)
	} else {
		s.pubsub = bus.NewFakePubSub()
	}

	// Initialize MQTT unless skipped
	if cfg.MQTTBroker != "skip" {
		opts := mqtt.NewClientOptions().AddBroker(cfg.MQTTBroker).SetClientID("subagent-" + cfg.TaskID)
		mclient := mqtt.NewClient(opts)
		if token := mclient.Connect(); token.Wait() && token.Error() != nil {
			return nil, fmt.Errorf("mqtt connection failed: %w", token.Error())
		}
		s.heartbeat = bus.NewMQTTHeartbeatBus(mclient)
	} else {
		s.heartbeat = bus.NewFakeHeartbeatBus()
	}

	return s, nil
}

func (s *Subagent) startHeartbeat(ctx context.Context) {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.heartbeat.PublishHeartbeat(ctx, s.cfg.TaskID, "running"); err != nil {
				fmt.Fprintf(os.Stderr, "Heartbeat failed: %v\n", err)
			}
		}
	}
}

func (s *Subagent) Run(ctx context.Context) error {
	// 启动心跳
	go s.startHeartbeat(ctx)
	// 立即发送一次初始心跳
	_ = s.heartbeat.PublishHeartbeat(ctx, s.cfg.TaskID, "running")

	// 发送启动消息
	startMsg := bus.Message{
		MessageID:  "subagent.started." + s.cfg.TaskID,
		SenderID:   s.cfg.TaskID,
		ReceiverID: "orchestrator",
		Payload:    json.RawMessage(`{"status":"started","agent_type":"` + s.cfg.AgentType + `"}`),
		Timestamp:  time.Now().UnixMilli(),
	}
	if err := s.pubsub.Publish(ctx, "subagent.events", startMsg); err != nil {
		return err
	}

	// 消费任务内容
	fmt.Printf("Agent [%s] (ID: %s) waiting for task...\n", s.cfg.AgentType, s.cfg.TaskID)
	taskChannel := "task." + s.cfg.TaskID
	msg, err := s.pubsub.Consume(ctx, taskChannel)
	if err != nil {
		return fmt.Errorf("failed to consume task: %w", err)
	}

	fmt.Printf("[%s] Executing task: %s\n", s.cfg.AgentType, string(msg.Payload))

	// 模拟解析 @agent (简单的正则或字符串查找)
	// 在真实场景中，这里会由 LLM 完成
	payloadStr := string(msg.Payload)
	if s.cfg.AgentType == "orchestrator" && (strings.Contains(payloadStr, "@researcher") || strings.Contains(payloadStr, "@coder")) {
		target := "researcher"
		if strings.Contains(payloadStr, "@coder") {
			target = "coder"
		}
		
		fmt.Printf("[%s] Delegating subtask to @%s...\n", s.cfg.AgentType, target)
		subTaskID := fmt.Sprintf("%s.sub.%s", s.cfg.TaskID, target)
		subMsg := bus.Message{
			MessageID:    subTaskID,
			ParentTaskID: s.cfg.TaskID,
			SenderID:     s.cfg.TaskID,
			ReceiverID:   "orchestrator",
			TargetAgent:  target,
			Payload:      json.RawMessage(`{"task":"Subtask for ` + target + `"}`),
			Timestamp:    time.Now().UnixMilli(),
		}
		
		// 发送到主 tasks 频道让 Orchestrator 调度
		_ = s.pubsub.Publish(ctx, "tasks", subMsg)
		
		// 模拟等待子任务完成 (实际应通过监听或轮询)
		time.Sleep(1 * time.Second)
	}

	// 模拟执行
	time.Sleep(2 * time.Second)

	// 发送结果
	result := bus.TaskResult{
		TaskID:       s.cfg.TaskID,
		ParentTaskID: msg.ParentTaskID,
		AgentName:    s.cfg.AgentType,
		Status:       "success",
		Output:       json.RawMessage(`{"result":"Task completed by ` + s.cfg.AgentType + `"}`),
		Timestamp:    time.Now().Unix(),
	}
	resPayload, _ := json.Marshal(result)
	resMsg := bus.Message{
		MessageID:  "result." + s.cfg.TaskID,
		SenderID:   s.cfg.TaskID,
		ReceiverID: "orchestrator",
		Payload:    resPayload,
		Timestamp:  time.Now().UnixMilli(),
	}

	if rps, ok := s.pubsub.(*bus.RedisPubSub); ok {
		_ = rps.Broadcast(ctx, "task_results", resMsg)
	} else {
		_ = s.pubsub.Publish(ctx, "task_results", resMsg)
	}

	fmt.Printf("[%s] Task %s completed.\n", s.cfg.AgentType, s.cfg.TaskID)
	return nil
}

func runMain(ctx context.Context, args []string) error {
	cfg, err := ParseConfig(args)
	if err != nil {
		return err
	}
	agent, err := NewSubagent(ctx, cfg)
	if err != nil {
		return err
	}
	return agent.Run(ctx)
}

func main() {
	if err := runMain(context.Background(), os.Args[1:]); err != nil {
		os.Exit(1)
	}
}
