package session

import (
	"context"
	"database/sql"
	"time"
)

type ChatMessage struct {
	ID        string `json:"id"`
	SessionID string `json:"session_id"`
	Role      string `json:"role"`    // system, user, assistant, tool
	Content   string `json:"content"`
	Tokens    int    `json:"tokens"`  // 预留，用于 Context Window Guard
	CreatedAt int64  `json:"created_at"`
}

type HistoryStore interface {
	AddMessage(ctx context.Context, msg ChatMessage) error
	GetHistory(ctx context.Context, sessionID string, limit int) ([]ChatMessage, error)
	ClearHistory(ctx context.Context, sessionID string) error
	InitSchema(ctx context.Context) error
}

type SQLiteHistoryStore struct {
	db *sql.DB
}

func NewSQLiteHistoryStore(db *sql.DB) *SQLiteHistoryStore {
	return &SQLiteHistoryStore{db: db}
}

func (s *SQLiteHistoryStore) InitSchema(ctx context.Context) error {
	q := `
	CREATE TABLE IF NOT EXISTS session_history (
		id TEXT PRIMARY KEY,
		session_id TEXT NOT NULL,
		role TEXT NOT NULL,
		content TEXT NOT NULL,
		tokens INTEGER DEFAULT 0,
		created_at INTEGER NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_session_id_time ON session_history(session_id, created_at ASC);
	`
	_, err := s.db.ExecContext(ctx, q)
	return err
}

func (s *SQLiteHistoryStore) AddMessage(ctx context.Context, msg ChatMessage) error {
	if msg.CreatedAt == 0 {
		msg.CreatedAt = time.Now().UnixMilli()
	}
	q := `INSERT INTO session_history (id, session_id, role, content, tokens, created_at) VALUES (?, ?, ?, ?, ?, ?)`
	_, err := s.db.ExecContext(ctx, q, msg.ID, msg.SessionID, msg.Role, msg.Content, msg.Tokens, msg.CreatedAt)
	return err
}

func (s *SQLiteHistoryStore) GetHistory(ctx context.Context, sessionID string, limit int) ([]ChatMessage, error) {
	// 获取最近的 limit 条消息
	q := `
		SELECT id, session_id, role, content, tokens, created_at 
		FROM session_history 
		WHERE session_id = ? 
		ORDER BY created_at DESC 
		LIMIT ?
	`
	rows, err := s.db.QueryContext(ctx, q, sessionID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []ChatMessage
	for rows.Next() {
		var m ChatMessage
		if err := rows.Scan(&m.ID, &m.SessionID, &m.Role, &m.Content, &m.Tokens, &m.CreatedAt); err != nil {
			return nil, err
		}
		messages = append(messages, m)
	}

	// 逆序，使其按时间正序排列 (从旧到新)
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}

	return messages, nil
}

func (s *SQLiteHistoryStore) ClearHistory(ctx context.Context, sessionID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM session_history WHERE session_id = ?`, sessionID)
	return err
}
