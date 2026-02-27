package models

import (
	"time"

	surrealmodels "github.com/surrealdb/surrealdb.go/pkg/models"
)

// RelationType describes the kind of relationship between documents.
type RelationType string

const (
	RelRelatesTo RelationType = "relates_to"
)

// RelationSource describes how a relation was created.
type RelationSource string

const (
	RelSourceFrontmatter RelationSource = "frontmatter"
	RelSourceAPI         RelationSource = "api"
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
