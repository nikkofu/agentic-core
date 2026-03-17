package memory

import (
	"context"
	"fmt"
	"time"

	"github.com/milvus-io/milvus-sdk-go/v2/client"
	"github.com/milvus-io/milvus-sdk-go/v2/entity"
)

const (
	DefaultCollectionName = "agent_memories"
	DefaultVectorDim      = 1536
)

type MilvusStore struct {
	client     client.Client
	collection string
}

func NewMilvusStore(ctx context.Context, addr string, collection string) (*MilvusStore, error) {
	c, err := client.NewClient(ctx, client.Config{
		Address: addr,
	})
	if err != nil {
		return nil, err
	}

	if collection == "" {
		collection = DefaultCollectionName
	}

	s := &MilvusStore{
		client:     c,
		collection: collection,
	}

	if err := s.ensureCollection(ctx); err != nil {
		_ = c.Close()
		return nil, err
	}

	return s, nil
}

func (s *MilvusStore) ensureCollection(ctx context.Context) error {
	exists, err := s.client.HasCollection(ctx, s.collection)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}

	schema := entity.NewSchema().WithName(s.collection).WithDescription("Agent long term memory").
		WithField(entity.NewField().WithName("id").WithDataType(entity.FieldTypeInt64).WithIsPrimaryKey(true).WithIsAutoID(true)).
		WithField(entity.NewField().WithName("task_id").WithDataType(entity.FieldTypeVarChar).WithMaxLength(128)).
		WithField(entity.NewField().WithName("content").WithDataType(entity.FieldTypeVarChar).WithMaxLength(65535)).
		WithField(entity.NewField().WithName("vector").WithDataType(entity.FieldTypeFloatVector).WithDim(DefaultVectorDim)).
		WithField(entity.NewField().WithName("timestamp").WithDataType(entity.FieldTypeInt64))

	if err := s.client.CreateCollection(ctx, schema, entity.DefaultShardNumber); err != nil {
		return err
	}

	// 创建索引
	idx, err := entity.NewIndexIvfFlat(entity.L2, 1024)
	if err != nil {
		return err
	}
	if err := s.client.CreateIndex(ctx, s.collection, "vector", idx, false); err != nil {
		return err
	}

	return s.client.LoadCollection(ctx, s.collection, false)
}

func (s *MilvusStore) Store(ctx context.Context, entry MemoryEntry) error {
	taskIDs := []string{entry.TaskID}
	contents := []string{entry.Content}
	vectors := [][]float32{entry.Vector}
	timestamps := []int64{entry.Timestamp}
	if timestamps[0] == 0 {
		timestamps[0] = time.Now().Unix()
	}

	taskIDCol := entity.NewColumnVarChar("task_id", taskIDs)
	contentCol := entity.NewColumnVarChar("content", contents)
	vectorCol := entity.NewColumnFloatVector("vector", DefaultVectorDim, vectors)
	timestampCol := entity.NewColumnInt64("timestamp", timestamps)

	_, err := s.client.Insert(ctx, s.collection, "", taskIDCol, contentCol, vectorCol, timestampCol)
	return err
}

func (s *MilvusStore) Search(ctx context.Context, vector []float32, limit int) ([]MemoryEntry, error) {
	searchParam, _ := entity.NewIndexIvfFlatSearchParam(10)
	
	results, err := s.client.Search(ctx, s.collection, nil, "", []string{"task_id", "content", "timestamp"},
		[]entity.Vector{entity.FloatVector(vector)}, "vector", entity.L2, limit, searchParam)
	if err != nil {
		return nil, err
	}

	var entries []MemoryEntry
	for _, res := range results {
		for i := 0; i < res.ResultCount; i++ {
			var entry MemoryEntry
			
			if id, err := res.IDs.GetAsInt64(i); err == nil {
				entry.ID = fmt.Sprintf("%d", id)
			}
			
			if taskID, err := res.Fields.GetColumn("task_id").GetAsString(i); err == nil {
				entry.TaskID = taskID
			}
			
			if content, err := res.Fields.GetColumn("content").GetAsString(i); err == nil {
				entry.Content = content
			}
			
			if timestamp, err := res.Fields.GetColumn("timestamp").GetAsInt64(i); err == nil {
				entry.Timestamp = timestamp
			}
			
			entries = append(entries, entry)
		}
	}

	return entries, nil
}

func (s *MilvusStore) Close() error {
	return s.client.Close()
}
