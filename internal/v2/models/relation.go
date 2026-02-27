package models

import (
	"time"

	surrealmodels "github.com/surrealdb/surrealdb.go/pkg/models"
)

type DocRelation struct {
	ID        surrealmodels.RecordID `json:"id"`
	In        surrealmodels.RecordID `json:"in"`
	Out       surrealmodels.RecordID `json:"out"`
	RelType   string                 `json:"rel_type"`
	Source    string                 `json:"source"`
	CreatedAt time.Time              `json:"created_at"`
}

type DocRelationInput struct {
	FromDocID string `json:"from_doc_id"`
	ToDocID   string `json:"to_doc_id"`
	RelType   string `json:"rel_type"`
	Source    string `json:"source"`
}
