package process

import "context"

type ProcessManager interface {
	SpawnAgent(ctx context.Context, agentType string, taskID string, extraArgs ...string) (pid int, err error)
	KillAgent(pid int) error
}
