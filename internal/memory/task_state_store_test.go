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
