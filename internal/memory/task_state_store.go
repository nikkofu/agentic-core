package memory

import (
	"context"
	"errors"
	"sync"
)

var ErrTaskStateNotFound = errors.New("task state not found")

type TaskState struct {
	TaskID        string
	ParentTaskID  string // 用于父子任务追踪
	AgentName     string // 执行任务的 Agent 名称
	Status        string // pending, running, success, failed, rejected, timeout, cancelled
	UpdatedAtUnix int64
	ErrorMessage  string
}

type AgentProfile struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	RolePrompt  string `json:"role_prompt"`
}

type TaskStateStore interface {
	Save(ctx context.Context, state TaskState) error
	Get(ctx context.Context, taskID string) (TaskState, error)
	GetSubTasks(ctx context.Context, parentTaskID string) ([]TaskState, error)
}

type InMemoryTaskStateStore struct {
	mu     sync.RWMutex
	states map[string]TaskState
}

func NewInMemoryTaskStateStore() *InMemoryTaskStateStore {
	return &InMemoryTaskStateStore{states: make(map[string]TaskState)}
}

func (s *InMemoryTaskStateStore) Save(ctx context.Context, state TaskState) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	s.states[state.TaskID] = state
	s.mu.Unlock()
	return nil
}

func (s *InMemoryTaskStateStore) Get(ctx context.Context, taskID string) (TaskState, error) {
	if err := ctx.Err(); err != nil {
		return TaskState{}, err
	}
	s.mu.RLock()
	state, ok := s.states[taskID]
	s.mu.RUnlock()
	if !ok {
		return TaskState{}, ErrTaskStateNotFound
	}
	return state, nil
}

func (s *InMemoryTaskStateStore) GetSubTasks(ctx context.Context, parentTaskID string) ([]TaskState, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var results []TaskState
	for _, state := range s.states {
		if state.ParentTaskID == parentTaskID {
			results = append(results, state)
		}
	}
	return results, nil
}
