package memory

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	_ "modernc.org/sqlite"
)

func TestSQLiteTaskStateStoreSaveAndGet(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	defer db.Close()

	store := NewSQLiteTaskStateStore(db)
	if err := store.InitSchema(context.Background()); err != nil {
		t.Fatalf("init schema failed: %v", err)
	}

	state := TaskState{TaskID: "task-1", Status: "running", UpdatedAtUnix: 1735689600}
	if err := store.Save(context.Background(), state); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	got, err := store.Get(context.Background(), "task-1")
	if err != nil {
		t.Fatalf("get failed: %v", err)
	}
	if got.TaskID != state.TaskID || got.Status != state.Status || got.UpdatedAtUnix != state.UpdatedAtUnix {
		t.Fatalf("unexpected state: %+v", got)
	}
}

func TestSQLiteTaskStateStoreGetNotFound(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	defer db.Close()

	store := NewSQLiteTaskStateStore(db)
	if err := store.InitSchema(context.Background()); err != nil {
		t.Fatalf("init schema failed: %v", err)
	}

	_, err = store.Get(context.Background(), "missing")
	if !errors.Is(err, ErrTaskStateNotFound) {
		t.Fatalf("expected ErrTaskStateNotFound, got %v", err)
	}
}
