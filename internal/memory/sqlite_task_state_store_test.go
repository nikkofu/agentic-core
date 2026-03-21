package memory

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
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

func TestSQLiteTaskStateStoreListRecoverableReturnsPendingAndRunning(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	defer db.Close()

	store := NewSQLiteTaskStateStore(db)
	if err := store.InitSchema(context.Background()); err != nil {
		t.Fatalf("init schema failed: %v", err)
	}

	states := []TaskState{
		{TaskID: "task-pending", Status: "pending", UpdatedAtUnix: 1},
		{TaskID: "task-running", Status: "running", UpdatedAtUnix: 2},
		{TaskID: "task-success", Status: "success", UpdatedAtUnix: 3},
	}
	for _, state := range states {
		if err := store.Save(context.Background(), state); err != nil {
			t.Fatalf("save failed: %v", err)
		}
	}

	got, err := store.ListRecoverable(context.Background())
	if err != nil {
		t.Fatalf("list recoverable failed: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("expected 2 recoverable states, got %d: %+v", len(got), got)
	}
	if got[0].Status != "pending" || got[1].Status != "running" {
		t.Fatalf("unexpected recoverable states: %+v", got)
	}
}

func TestSQLiteTaskStateStoreListRecoverableExcludesTerminalStatuses(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	defer db.Close()

	store := NewSQLiteTaskStateStore(db)
	if err := store.InitSchema(context.Background()); err != nil {
		t.Fatalf("init schema failed: %v", err)
	}

	states := []TaskState{
		{TaskID: "task-success", Status: "success", UpdatedAtUnix: 1},
		{TaskID: "task-failed", Status: "failed", UpdatedAtUnix: 2},
		{TaskID: "task-cancelled", Status: "cancelled", UpdatedAtUnix: 3},
		{TaskID: "task-pending", Status: "pending", UpdatedAtUnix: 4},
	}
	for _, state := range states {
		if err := store.Save(context.Background(), state); err != nil {
			t.Fatalf("save failed: %v", err)
		}
	}

	got, err := store.ListRecoverable(context.Background())
	if err != nil {
		t.Fatalf("list recoverable failed: %v", err)
	}

	wantStatuses := []string{"pending"}
	gotStatuses := make([]string, 0, len(got))
	for _, state := range got {
		gotStatuses = append(gotStatuses, state.Status)
	}
	if !reflect.DeepEqual(gotStatuses, wantStatuses) {
		t.Fatalf("unexpected recoverable statuses: got %v want %v", gotStatuses, wantStatuses)
	}
}
