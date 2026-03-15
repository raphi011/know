package models

import surrealmodels "github.com/surrealdb/surrealdb.go/pkg/models"

type ExternalLink struct {
	ID       surrealmodels.RecordID `json:"id"`
	FromFile surrealmodels.RecordID `json:"from_file"`
	Vault    surrealmodels.RecordID `json:"vault"`
	Hostname string                 `json:"hostname"`
	URLPath  string                 `json:"url_path"`
	FullURL  string                 `json:"full_url"`
	LinkText *string                `json:"link_text,omitempty"`
}
