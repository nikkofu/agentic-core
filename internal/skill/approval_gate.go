package skill

import (
	"agentic-core/internal/bus"
	"agentic-core/internal/llm"
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// ApprovalGate 负责处理 Human-in-the-loop 审批流
type ApprovalGate struct {
	ps bus.PubSub
}

func NewApprovalGate(ps bus.PubSub) *ApprovalGate {
	return &ApprovalGate{ps: ps}
}

// WaitDecision 等待用户审批决策
func (g *ApprovalGate) WaitDecision(ctx context.Context, req llm.ApprovalRequest, timeout time.Duration) (llm.ApprovalDecision, error) {
	// 1. 订阅审批结果频道
	// 实际上我们可能需要一个全局订阅者或者按任务订阅
	// 在 MVP 中，我们通过通配符或特定任务频道监听
	msgChan, err := g.ps.Consume(ctx, "approvals")
	if err != nil {
		return llm.ApprovalDecision{}, err
	}

	// 2. 设置超时
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return llm.ApprovalDecision{}, ctx.Err()
		case <-timer.C:
			return llm.ApprovalDecision{
				TraceID:    req.TraceID,
				TaskID:     req.TaskID,
				ToolCallID: req.ToolCallID,
				Approved:   false,
				Reason:     "timeout",
			}, fmt.Errorf("approval timeout")
		case msg, ok := <-msgChan:
			if !ok {
				return llm.ApprovalDecision{}, fmt.Errorf("approval channel closed")
			}

			var decision llm.ApprovalDecision
			if err := json.Unmarshal(msg.Payload, &decision); err != nil {
				continue
			}

			// 3. 匹配决策 (幂等键: TaskID + ToolCallID)
			if decision.TaskID == req.TaskID && decision.ToolCallID == req.ToolCallID {
				return decision, nil
			}
		}
	}
}
