package memory

import (
	"context"
	"database/sql"
)

type SQLiteTaskStateStore struct {
	db *sql.DB
}

func NewSQLiteTaskStateStore(db *sql.DB) *SQLiteTaskStateStore {
	return &SQLiteTaskStateStore{db: db}
}

func (s *SQLiteTaskStateStore) InitSchema(ctx context.Context) error {
	q := `
	CREATE TABLE IF NOT EXISTS task_states (
		task_id TEXT PRIMARY KEY,
		parent_task_id TEXT,
		agent_name TEXT,
		status TEXT,
		updated_at_unix INTEGER,
		error_message TEXT
	);
	CREATE INDEX IF NOT EXISTS idx_parent_task_id ON task_states(parent_task_id);
	`
	_, err := s.db.ExecContext(ctx, q)
	return err
}

func (s *SQLiteTaskStateStore) Save(ctx context.Context, state TaskState) error {
	q := `
	INSERT INTO task_states (task_id, parent_task_id, agent_name, status, updated_at_unix, error_message)
	VALUES (?, ?, ?, ?, ?, ?)
	ON CONFLICT(task_id) DO UPDATE SET
		status=excluded.status,
		updated_at_unix=excluded.updated_at_unix,
		error_message=excluded.error_message;
	`
	_, err := s.db.ExecContext(ctx, q,
		state.TaskID, state.ParentTaskID, state.AgentName, state.Status, state.UpdatedAtUnix, state.ErrorMessage,
	)
	return err
}

func (s *SQLiteTaskStateStore) Get(ctx context.Context, taskID string) (TaskState, error) {
	q := `SELECT task_id, parent_task_id, agent_name, status, updated_at_unix, error_message FROM task_states WHERE task_id = ?`
	row := s.db.QueryRowContext(ctx, q, taskID)

	var state TaskState
	var parentID, agentName sql.NullString
	err := row.Scan(&state.TaskID, &parentID, &agentName, &state.Status, &state.UpdatedAtUnix, &state.ErrorMessage)
	if err != nil {
		if err == sql.ErrNoRows {
			return TaskState{}, ErrTaskStateNotFound
		}
		return TaskState{}, err
	}
	state.ParentTaskID = parentID.String
	state.AgentName = agentName.String
	return state, nil
}

func (s *SQLiteTaskStateStore) GetSubTasks(ctx context.Context, parentTaskID string) ([]TaskState, error) {
	q := `SELECT task_id, parent_task_id, agent_name, status, updated_at_unix, error_message FROM task_states WHERE parent_task_id = ?`
	rows, err := s.db.QueryContext(ctx, q, parentTaskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []TaskState
	for rows.Next() {
		var state TaskState
		var parentID, agentName sql.NullString
		if err := rows.Scan(&state.TaskID, &parentID, &agentName, &state.Status, &state.UpdatedAtUnix, &state.ErrorMessage); err != nil {
			return nil, err
		}
		state.ParentTaskID = parentID.String
		state.AgentName = agentName.String
		results = append(results, state)
	}
	return results, rows.Err()
}
