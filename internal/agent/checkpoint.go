package agent

import (
	"context"

	"github.com/cloudwego/eino/adk"
	"github.com/raphi011/know/internal/db"
)

// Compile-time interface assertion.
var _ adk.CheckPointStore = (*SurrealCheckPointStore)(nil)

// SurrealCheckPointStore implements adk.CheckPointStore backed by SurrealDB.
type SurrealCheckPointStore struct {
	db *db.Client
}

// NewCheckPointStore creates a new SurrealDB-backed checkpoint store.
func NewCheckPointStore(db *db.Client) *SurrealCheckPointStore {
	return &SurrealCheckPointStore{db: db}
}

func (s *SurrealCheckPointStore) Get(ctx context.Context, id string) ([]byte, bool, error) {
	data, err := s.db.GetCheckpoint(ctx, id)
	if err != nil {
		return nil, false, err
	}
	if data == nil {
		return nil, false, nil
	}
	return data, true, nil
}

func (s *SurrealCheckPointStore) Set(ctx context.Context, id string, data []byte) error {
	return s.db.UpsertCheckpoint(ctx, id, data)
}
