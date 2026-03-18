package llm

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ToolExecutor 定义了工具执行接口，解耦 internal/skill
type ToolExecutor interface {
	Execute(ctx context.Context, call ToolCall) (ToolResult, error)
}

// ApprovalGate 定义了审批门禁接口
type ApprovalGate interface {
	WaitDecision(ctx context.Context, req ApprovalRequest, timeout time.Duration) (ApprovalDecision, error)
}

// Runtime 负责驱动 ReAct 循环
type Runtime struct {
	Provider     Provider
	Executor     ToolExecutor
	ApprovalGate ApprovalGate
	MaxTurns     int
}

func NewRuntime(p Provider, e ToolExecutor, g ApprovalGate) *Runtime {
	return &Runtime{
		Provider:     p,
		Executor:     e,
		ApprovalGate: g,
		MaxTurns:     10,
	}
}

// Run 执行完整的智能体推理循环
func (r *Runtime) Run(ctx context.Context, req InferenceRequest, fanout *Fanout) (FinalResult, error) {
	messages := req.Messages

	for turn := 0; turn < r.MaxTurns; turn++ {
		// 1. LLM 推理
		resp, err := r.Provider.Predict(ctx, InferenceRequest{
			TraceID:     req.TraceID,
			Messages:    messages,
			ModelAlias:  req.ModelAlias,
			Temperature: req.Temperature,
		})
		if err != nil {
			return r.failWithTerminalEvent(ctx, req, fanout, err)
		}

		// 2. 解析输出
		action, err := ParseModelOutput(resp)
		if err != nil {
			return r.failWithTerminalEvent(ctx, req, fanout, err)
		}

		// 3. 判断是否结束
		if action.Final != "" {
			if fanout != nil {
				_ = fanout.EmitFinal(ctx, action.Final)
			}
			return FinalResult{
				TraceID:     req.TraceID,
				TaskID:      req.TaskID,
				Content:     action.Final,
				Status:      "success",
				TimestampMs: time.Now().UnixMilli(),
			}, nil
		}

		// 4. 处理工具调用
		if action.CallSkill != nil {
			call := *action.CallSkill
			if fanout != nil {
				_ = fanout.EmitToolCall(ctx, call)
			}

			// 4.1 检查是否需要审批
			if call.IsWriteOperation && r.ApprovalGate == nil {
				return r.failWithTerminalEvent(ctx, req, fanout, fmt.Errorf("approval gate required for write operation"))
			}
			if call.IsWriteOperation && r.ApprovalGate != nil {
				appReq := ApprovalRequest{
					TraceID:       req.TraceID,
					TaskID:        req.TaskID,
					ToolCallID:    call.ID,
					ToolName:      call.Name,
					Arguments:     call.Arguments,
					RequestedAtMs: time.Now().UnixMilli(),
				}
				if fanout != nil {
					_ = fanout.EmitWaitingApproval(ctx, appReq)
				}
				decision, err := r.ApprovalGate.WaitDecision(ctx, appReq, 5*time.Minute)
				if err != nil {
					return r.failWithTerminalEvent(ctx, req, fanout, err)
				}
				if !decision.Approved {
					res := ToolResult{
						ToolCallID: call.ID,
						Name:       call.Name,
						Success:    false,
						Output:     "User rejected this operation: " + decision.Reason,
						Error:      decision.Reason,
					}
					if fanout != nil {
						_ = fanout.EmitToolResult(ctx, res)
					}
					// 根据策略处理拒绝
					if req.OnApprovalReject == "fail" {
						if fanout != nil {
							_ = fanout.EmitError(ctx, "approval rejected")
						}
						return FinalResult{Status: "rejected"}, fmt.Errorf("approval rejected")
					}
					// 默认 continue: 回注拒绝信息
					messages = append(messages, ChatMessage{Role: "assistant", Content: resp})
					messages = append(messages, ChatMessage{Role: "tool", ToolCallID: call.ID, Content: res.Output})
					continue
				}
			}

			// 4.2 执行工具
			res, err := r.Executor.Execute(ctx, call)
			if err != nil {
				// 执行失败也作为 Observation 回注
				res = ToolResult{
					ToolCallID: call.ID,
					Name:       call.Name,
					Success:    false,
					Output:     err.Error(),
					Error:      err.Error(),
				}
			}
			if fanout != nil {
				_ = fanout.EmitToolResult(ctx, res)
			}

			// 4.3 回注 Observation
			messages = append(messages, ChatMessage{Role: "assistant", Content: resp})
			messages = append(messages, ChatMessage{Role: "tool", ToolCallID: call.ID, Content: res.Output})
		}
	}

	err := fmt.Errorf("exceeded max turns")
	return r.failWithTerminalEvent(ctx, req, fanout, err)
}

func (r *Runtime) failWithTerminalEvent(ctx context.Context, req InferenceRequest, fanout *Fanout, err error) (FinalResult, error) {
	status, emit := classifyTerminalError(err)
	if fanout != nil {
		switch emit {
		case "cancelled":
			_ = fanout.EmitCancelled(ctx, err.Error())
		case "timeout":
			_ = fanout.EmitTimeout(ctx, err.Error())
		default:
			_ = fanout.EmitError(ctx, err.Error())
		}
	}
	return FinalResult{
		TraceID:     req.TraceID,
		TaskID:      req.TaskID,
		Status:      status,
		TimestampMs: time.Now().UnixMilli(),
	}, err
}

func classifyTerminalError(err error) (status string, event string) {
	errLower := strings.ToLower(err.Error())
	switch {
	case errors.Is(err, context.Canceled):
		return "cancelled", "cancelled"
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout", "timeout"
	case strings.Contains(errLower, "exceeded max turns"):
		return "failed", "error"
	case strings.Contains(errLower, "approval gate required"):
		return "failed", "error"
	case strings.Contains(errLower, "timeout"):
		return "timeout", "timeout"
	default:
		return "error", "error"
	}
}
