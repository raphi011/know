package db

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestLookupQueryEmbedding_NotFound(t *testing.T) {
	ctx := context.Background()

	query := fmt.Sprintf("nonexistent-query-%d", time.Now().UnixNano())
	result, err := testDB.LookupQueryEmbedding(ctx, query)
	if err != nil {
		t.Fatalf("LookupQueryEmbedding failed: %v", err)
	}
	if result != nil {
		t.Error("Expected nil for nonexistent query embedding")
	}
}

func TestUpsertAndLookupQueryEmbedding(t *testing.T) {
	ctx := context.Background()

	query := fmt.Sprintf("test query %d", time.Now().UnixNano())
	embedding := dummyEmbedding()

	err := testDB.UpsertQueryEmbedding(ctx, query, embedding)
	if err != nil {
		t.Fatalf("UpsertQueryEmbedding failed: %v", err)
	}

	result, err := testDB.LookupQueryEmbedding(ctx, query)
	if err != nil {
		t.Fatalf("LookupQueryEmbedding failed: %v", err)
	}
	if result == nil {
		t.Fatal("LookupQueryEmbedding returned nil after upsert")
	}
	if len(result.Embedding) != 384 {
		t.Errorf("Expected embedding length 384, got %d", len(result.Embedding))
	}
	if result.Query != query {
		t.Errorf("Expected query %q, got %q", query, result.Query)
	}
}

func TestUpsertQueryEmbedding_UpdatesExisting(t *testing.T) {
	ctx := context.Background()

	query := fmt.Sprintf("update query %d", time.Now().UnixNano())
	embedding1 := dummyEmbedding()

	err := testDB.UpsertQueryEmbedding(ctx, query, embedding1)
	if err != nil {
		t.Fatalf("First UpsertQueryEmbedding failed: %v", err)
	}

	// Upsert again with same query
	embedding2 := dummyEmbedding()
	err = testDB.UpsertQueryEmbedding(ctx, query, embedding2)
	if err != nil {
		t.Fatalf("Second UpsertQueryEmbedding failed: %v", err)
	}

	result, err := testDB.LookupQueryEmbedding(ctx, query)
	if err != nil {
		t.Fatalf("LookupQueryEmbedding after double upsert failed: %v", err)
	}
	if result == nil {
		t.Fatal("LookupQueryEmbedding returned nil after double upsert")
	}
	if len(result.Embedding) != 384 {
		t.Errorf("Expected embedding length 384, got %d", len(result.Embedding))
	}
}
