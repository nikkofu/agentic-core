package memory

import (
	"context"
	"testing"
)

func TestInMemoryTaskStateStoreSaveAndGet(t *testing.T) {
	store := NewInMemoryTaskStateStore()
	ctx := context.Background()

	state := TaskState{TaskID: "task-1", Status: "running", UpdatedAtUnix: 1735689600}
	if err := store.Save(ctx, state); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	got, err := store.Get(ctx, "task-1")
	if err != nil {
		t.Fatalf("get failed: %v", err)
	}
	if got.TaskID != state.TaskID || got.Status != state.Status || got.UpdatedAtUnix != state.UpdatedAtUnix {
		t.Fatalf("unexpected state: %+v", got)
	}
}

func TestInMemoryTaskStateStoreGetNotFound(t *testing.T) {
	store := NewInMemoryTaskStateStore()
	_, err := store.Get(context.Background(), "missing")
	if err == nil {
		t.Fatal("expected not found error")
	}
}

func TestInMemoryTaskStateStoreListRecoverableReturnsPendingAndRunning(t *testing.T) {
	store := NewInMemoryTaskStateStore()
	ctx := context.Background()

	states := []TaskState{
		{TaskID: "task-pending", Status: "pending"},
		{TaskID: "task-running", Status: "running"},
		{TaskID: "task-success", Status: "success"},
	}

	for _, state := range states {
		if err := store.Save(ctx, state); err != nil {
			t.Fatalf("save failed: %v", err)
		}
	}

	got, err := store.ListRecoverable(ctx)
	if err != nil {
		t.Fatalf("list recoverable failed: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 recoverable tasks, got %d: %+v", len(got), got)
	}

	want := map[string]string{
		"task-pending": "pending",
		"task-running": "running",
	}
	for _, state := range got {
		if gotStatus, ok := want[state.TaskID]; !ok || gotStatus != state.Status {
			t.Fatalf("unexpected recoverable state: %+v", state)
		}
	}
}

func TestInMemoryTaskStateStoreListRecoverableExcludesTerminalStatuses(t *testing.T) {
	store := NewInMemoryTaskStateStore()
	ctx := context.Background()

	states := []TaskState{
		{TaskID: "task-success", Status: "success"},
		{TaskID: "task-failed", Status: "failed"},
		{TaskID: "task-rejected", Status: "rejected"},
		{TaskID: "task-timeout", Status: "timeout"},
		{TaskID: "task-cancelled", Status: "cancelled"},
	}

	for _, state := range states {
		if err := store.Save(ctx, state); err != nil {
			t.Fatalf("save failed: %v", err)
		}
	}

	got, err := store.ListRecoverable(ctx)
	if err != nil {
		t.Fatalf("list recoverable failed: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected no recoverable tasks, got %+v", got)
	}
}
