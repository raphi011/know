package models

import (
	"time"

	surrealmodels "github.com/surrealdb/surrealdb.go/pkg/models"
)

// RelationType describes the kind of relationship between files.
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

type FileRelation struct {
	ID        surrealmodels.RecordID `json:"id"`
	In        surrealmodels.RecordID `json:"in"`
	Out       surrealmodels.RecordID `json:"out"`
	RelType   string                 `json:"rel_type"`
	Source    string                 `json:"source"`
	CreatedAt time.Time              `json:"created_at"`
}

type FileRelationInput struct {
	FromFileID string `json:"from_file_id"`
	ToFileID   string `json:"to_file_id"`
	RelType    string `json:"rel_type"`
	Source     string `json:"source"`
}
