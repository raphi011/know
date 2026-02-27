package models

import surrealmodels "github.com/surrealdb/surrealdb.go/pkg/models"

type WikiLink struct {
	ID        surrealmodels.RecordID  `json:"id"`
	FromDoc   surrealmodels.RecordID  `json:"from_doc"`
	ToDoc     *surrealmodels.RecordID `json:"to_doc,omitempty"`
	RawTarget string                  `json:"raw_target"`
	Vault     surrealmodels.RecordID  `json:"vault"`
}
