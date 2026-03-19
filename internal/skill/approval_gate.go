package skill

import (
	"agentic-core/internal/bus"
	"agentic-core/internal/llm"
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// ApprovalGate 负责处理 Human-in-the-loop 审批流
type ApprovalGate struct {
	events bus.EventBus

	mu       sync.Mutex
	pending  map[string][]llm.ApprovalDecision
	waiters  map[string][]approvalWaiter
	startErr error
}

type approvalWaiter struct {
	traceID string
	ch      chan llm.ApprovalDecision
}

func NewApprovalGate(events bus.EventBus) *ApprovalGate {
	gate := &ApprovalGate{
		events:  events,
		pending: make(map[string][]llm.ApprovalDecision),
		waiters: make(map[string][]approvalWaiter),
	}
	if events == nil {
		gate.startErr = fmt.Errorf("approval event bus is required")
		return gate
	}

	msgChan, err := events.Subscribe(context.Background(), "approvals")
	if err != nil {
		gate.startErr = err
		return gate
	}

	go gate.consume(msgChan)
	return gate
}

// WaitDecision 等待用户审批决策
func (g *ApprovalGate) WaitDecision(ctx context.Context, req llm.ApprovalRequest, timeout time.Duration) (llm.ApprovalDecision, error) {
	if err := req.Validate(); err != nil {
		return llm.ApprovalDecision{}, err
	}
	if g == nil {
		return llm.ApprovalDecision{}, fmt.Errorf("approval gate is required")
	}
	if g.startErr != nil {
		return llm.ApprovalDecision{}, g.startErr
	}

	key := approvalKey(req.TaskID, req.ToolCallID)
	if decision, ok := g.takePendingDecision(key, req.TraceID); ok {
		return decision, nil
	}

	waitCh := make(chan llm.ApprovalDecision, 1)
	g.addWaiter(key, approvalWaiter{traceID: req.TraceID, ch: waitCh})
	defer g.removeWaiter(key, waitCh)
	if decision, ok := g.takePendingDecision(key, req.TraceID); ok {
		return decision, nil
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
				TraceID:     req.TraceID,
				TaskID:      req.TaskID,
				ToolCallID:  req.ToolCallID,
				Approved:    false,
				Reason:      "timeout",
				DecidedAtMs: time.Now().UnixMilli(),
			}, fmt.Errorf("approval timeout")
		case decision := <-waitCh:
			return decision, nil
		}
	}
}

func (g *ApprovalGate) consume(msgChan <-chan bus.Message) {
	for msg := range msgChan {
		var decision llm.ApprovalDecision
		if err := json.Unmarshal(msg.Payload, &decision); err != nil {
			continue
		}
		if err := decision.Validate(); err != nil {
			continue
		}
		g.routeDecision(decision)
	}
}

func (g *ApprovalGate) routeDecision(decision llm.ApprovalDecision) {
	key := approvalKey(decision.TaskID, decision.ToolCallID)

	g.mu.Lock()
	defer g.mu.Unlock()

	waiters := g.waiters[key]
	if len(waiters) == 0 {
		g.pending[key] = append(g.pending[key], decision)
		return
	}

	remaining := waiters[:0]
	delivered := false
	for _, waiter := range waiters {
		if !approvalTraceMatches(waiter.traceID, decision.TraceID) {
			remaining = append(remaining, waiter)
			continue
		}
		select {
		case waiter.ch <- decision:
			delivered = true
		default:
		}
	}
	if len(remaining) == 0 {
		delete(g.waiters, key)
	} else {
		g.waiters[key] = remaining
	}
	if !delivered {
		g.pending[key] = append(g.pending[key], decision)
	}
}

func (g *ApprovalGate) addWaiter(key string, waiter approvalWaiter) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.waiters[key] = append(g.waiters[key], waiter)
}

func (g *ApprovalGate) removeWaiter(key string, waitCh chan llm.ApprovalDecision) {
	g.mu.Lock()
	defer g.mu.Unlock()

	waiters := g.waiters[key]
	if len(waiters) == 0 {
		return
	}

	filtered := waiters[:0]
	for _, waiter := range waiters {
		if waiter.ch == waitCh {
			continue
		}
		filtered = append(filtered, waiter)
	}
	if len(filtered) == 0 {
		delete(g.waiters, key)
		return
	}
	g.waiters[key] = filtered
}

func (g *ApprovalGate) takePendingDecision(key string, traceID string) (llm.ApprovalDecision, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()

	decisions := g.pending[key]
	if len(decisions) == 0 {
		return llm.ApprovalDecision{}, false
	}

	remaining := decisions[:0]
	for _, decision := range decisions {
		if !approvalTraceMatches(traceID, decision.TraceID) {
			remaining = append(remaining, decision)
			continue
		}
		if len(remaining) == 0 {
			delete(g.pending, key)
		} else {
			g.pending[key] = remaining
		}
		return decision, true
	}

	if len(remaining) == 0 {
		delete(g.pending, key)
	} else {
		g.pending[key] = remaining
	}
	return llm.ApprovalDecision{}, false
}

func approvalKey(taskID, toolCallID string) string {
	return taskID + ":" + toolCallID
}

func approvalTraceMatches(expected string, actual string) bool {
	return expected == "" || actual == "" || expected == actual
}

func (g *ApprovalGate) PendingDecisionCount() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	total := 0
	for _, decisions := range g.pending {
		total += len(decisions)
	}
	return total
}
