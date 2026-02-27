package graph

import (
	"fmt"

	"github.com/raphaelgruber/memcp-go/internal/v2/models"
	"github.com/raphaelgruber/memcp-go/internal/v2/search"
)

func vaultToGraphQL(v *models.Vault) *Vault {
	if v == nil {
		return nil
	}
	id, err := models.RecordIDString(v.ID)
	if err != nil {
		id = fmt.Sprintf("%v", v.ID.ID)
	}
	createdBy, err := models.RecordIDString(v.CreatedBy)
	if err != nil {
		createdBy = fmt.Sprintf("%v", v.CreatedBy.ID)
	}
	return &Vault{
		ID:          id,
		Name:        v.Name,
		Description: v.Description,
		CreatedBy:   createdBy,
		CreatedAt:   v.CreatedAt,
		UpdatedAt:   v.UpdatedAt,
	}
}

func documentToGraphQL(d *models.Document) *Document {
	if d == nil {
		return nil
	}
	id, err := models.RecordIDString(d.ID)
	if err != nil {
		id = fmt.Sprintf("%v", d.ID.ID)
	}
	vaultID, err := models.RecordIDString(d.Vault)
	if err != nil {
		vaultID = fmt.Sprintf("%v", d.Vault.ID)
	}
	labels := d.Labels
	if labels == nil {
		labels = []string{}
	}
	return &Document{
		ID:          id,
		VaultID:     vaultID,
		Path:        d.Path,
		Title:       d.Title,
		Content:     d.Content,
		ContentBody: d.ContentBody,
		Labels:      labels,
		DocType:     d.DocType,
		Source:      string(d.Source),
		SourcePath:  d.SourcePath,
		ContentHash: d.ContentHash,
		Metadata:    d.Metadata,
		CreatedAt:   d.CreatedAt,
		UpdatedAt:   d.UpdatedAt,
	}
}

func folderToGraphQL(f models.Folder) Folder {
	return Folder{
		Path:     f.Path,
		Name:     f.Name,
		DocCount: f.DocCount,
	}
}

func wikiLinkToGraphQL(l *models.WikiLink) *WikiLink {
	if l == nil {
		return nil
	}
	id, err := models.RecordIDString(l.ID)
	if err != nil {
		id = fmt.Sprintf("%v", l.ID.ID)
	}
	fromDocID, err := models.RecordIDString(l.FromDoc)
	if err != nil {
		fromDocID = fmt.Sprintf("%v", l.FromDoc.ID)
	}
	var toDocID *string
	if l.ToDoc != nil {
		s, err := models.RecordIDString(*l.ToDoc)
		if err != nil {
			s = fmt.Sprintf("%v", l.ToDoc.ID)
		}
		toDocID = &s
	}
	return &WikiLink{
		ID:        id,
		FromDocID: fromDocID,
		ToDocID:   toDocID,
		RawTarget: l.RawTarget,
		Resolved:  l.ToDoc != nil,
	}
}

func relationToGraphQL(r *models.DocRelation) *DocRelation {
	if r == nil {
		return nil
	}
	id, err := models.RecordIDString(r.ID)
	if err != nil {
		id = fmt.Sprintf("%v", r.ID.ID)
	}
	inID, err := models.RecordIDString(r.In)
	if err != nil {
		inID = fmt.Sprintf("%v", r.In.ID)
	}
	outID, err := models.RecordIDString(r.Out)
	if err != nil {
		outID = fmt.Sprintf("%v", r.Out.ID)
	}
	return &DocRelation{
		ID:        id,
		FromDocID: inID,
		ToDocID:   outID,
		RelType:   r.RelType,
		Source:     r.Source,
		CreatedAt: r.CreatedAt,
	}
}

func templateToGraphQL(t *models.Template) *Template {
	if t == nil {
		return nil
	}
	id, err := models.RecordIDString(t.ID)
	if err != nil {
		id = fmt.Sprintf("%v", t.ID.ID)
	}
	var vaultID *string
	if t.Vault != nil {
		s, err := models.RecordIDString(*t.Vault)
		if err != nil {
			s = fmt.Sprintf("%v", t.Vault.ID)
		}
		vaultID = &s
	}
	return &Template{
		ID:           id,
		VaultID:      vaultID,
		Name:         t.Name,
		Description:  t.Description,
		Content:      t.Content,
		IsAITemplate: t.IsAITemplate,
		CreatedAt:    t.CreatedAt,
		UpdatedAt:    t.UpdatedAt,
	}
}

func userToGraphQL(u *models.User) *User {
	if u == nil {
		return nil
	}
	id, err := models.RecordIDString(u.ID)
	if err != nil {
		id = fmt.Sprintf("%v", u.ID.ID)
	}
	return &User{
		ID:        id,
		Name:      u.Name,
		Email:     u.Email,
		CreatedAt: u.CreatedAt,
	}
}

func searchResultToGraphQL(r search.SearchResult) SearchResult {
	doc := documentToGraphQL(&r.Document)
	chunks := make([]ChunkMatch, len(r.MatchedChunks))
	for i, ch := range r.MatchedChunks {
		chunks[i] = ChunkMatch{
			Content:     ch.Content,
			HeadingPath: ch.HeadingPath,
			Position:    ch.Position,
			Score:       ch.Score,
		}
	}
	return SearchResult{
		Document:      *doc,
		Score:         r.Score,
		MatchedChunks: chunks,
	}
}
