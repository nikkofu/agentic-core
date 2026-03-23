package process

import (
	"context"
	"strings"
	"testing"
)

var _ ProcessManager = (*ExecProcessManager)(nil)

func TestExecProcessManagerSpawnAgentRequiresBinaryPath(t *testing.T) {
	pm := ExecProcessManager{}
	_, err := pm.SpawnAgent(context.Background(), "worker", "task-1")
	if err == nil {
		t.Fatal("expected error when binary path is empty")
	}
}

func TestExecProcessManagerSpawnAgentReturnsErrorForMissingBinary(t *testing.T) {
	pm := ExecProcessManager{BinaryPath: "/path/not/exist/subagent"}
	_, err := pm.SpawnAgent(context.Background(), "worker", "task-1")
	if err == nil {
		t.Fatal("expected error when binary does not exist")
	}
}

func TestExecProcessManagerKillAgentRejectsInvalidPID(t *testing.T) {
	pm := ExecProcessManager{}
	if err := pm.KillAgent(0); err == nil {
		t.Fatal("expected error for invalid pid")
	}
}

func TestExecProcessManagerKillAgentRejectsUntrackedPID(t *testing.T) {
	pm := ExecProcessManager{}
	err := pm.KillAgent(12345)
	if err == nil {
		t.Fatal("expected error for untracked pid")
	}
	if !strings.Contains(err.Error(), "untracked pid") {
		t.Fatalf("expected untracked pid error, got: %v", err)
	}
}
