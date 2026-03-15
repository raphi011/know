package models

import surrealmodels "github.com/surrealdb/surrealdb.go/pkg/models"

type WikiLink struct {
	ID        surrealmodels.RecordID  `json:"id"`
	FromFile  surrealmodels.RecordID  `json:"from_file"`
	ToFile    *surrealmodels.RecordID `json:"to_file,omitempty"`
	RawTarget string                  `json:"raw_target"`
	Vault     surrealmodels.RecordID  `json:"vault"`
}
