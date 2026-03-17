package memory

import "context"

type MemoryEntry struct {
	ID        string
	TaskID    string
	Content   string
	Vector    []float32
	Metadata  map[string]string
	Timestamp int64
}

type LongTermMemory interface {
	Store(ctx context.Context, entry MemoryEntry) error
	Search(ctx context.Context, vector []float32, limit int) ([]MemoryEntry, error)
}
