package process

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sync"
)

type ExecProcessManager struct {
	BinaryPath string

	mu      sync.Mutex
	tracked map[int]struct{}
}

func (m *ExecProcessManager) SpawnAgent(ctx context.Context, agentType string, taskID string) (pid int, err error) {
	if m.BinaryPath == "" {
		return 0, errors.New("binary path is required")
	}

	args := []string{"--agent-type", agentType, "--task-id", taskID}
	cmd := exec.CommandContext(ctx, m.BinaryPath, args...)
	if err := cmd.Start(); err != nil {
		return 0, err
	}

	m.mu.Lock()
	if m.tracked == nil {
		m.tracked = make(map[int]struct{})
	}
	m.tracked[cmd.Process.Pid] = struct{}{}
	m.mu.Unlock()

	return cmd.Process.Pid, nil
}

func (m *ExecProcessManager) KillAgent(pid int) error {
	if pid <= 0 {
		return errors.New("invalid pid")
	}

	m.mu.Lock()
	_, ok := m.tracked[pid]
	m.mu.Unlock()
	if !ok {
		return fmt.Errorf("untracked pid: %d", pid)
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	if err := proc.Kill(); err != nil {
		return err
	}

	m.mu.Lock()
	delete(m.tracked, pid)
	m.mu.Unlock()
	return nil
}
