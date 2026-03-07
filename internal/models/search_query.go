package models

import (
	"time"

	surrealmodels "github.com/surrealdb/surrealdb.go/pkg/models"
)

type SearchQuery struct {
	ID              surrealmodels.RecordID `json:"id"`
	Query           string                 `json:"query"`
	Embedding       []float32              `json:"embedding"`
	HitCount        int                    `json:"hit_count"`
	FirstSearchedAt time.Time              `json:"first_searched_at"`
	LastSearchedAt  time.Time              `json:"last_searched_at"`
}
