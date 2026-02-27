package graph

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/raphaelgruber/memcp-go/internal/parser"
	v2db "github.com/raphaelgruber/memcp-go/internal/v2/db"
	"github.com/raphaelgruber/memcp-go/internal/v2/models"
	"github.com/raphaelgruber/memcp-go/internal/v2/search"
)

func vaultToGraphQL(v *models.Vault) *Vault {
	if v == nil {
		return nil
	}
	id, err := models.RecordIDString(v.ID)
	if err != nil {
		slog.Warn("unexpected vault ID format in GraphQL conversion", "vault_name", v.Name, "error", err)
		id = fmt.Sprintf("%v", v.ID.ID)
	}
	createdBy, err := models.RecordIDString(v.CreatedBy)
	if err != nil {
		slog.Warn("unexpected vault createdBy format in GraphQL conversion", "vault_name", v.Name, "error", err)
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
		slog.Warn("unexpected document ID format in GraphQL conversion", "path", d.Path, "error", err)
		id = fmt.Sprintf("%v", d.ID.ID)
	}
	vaultID, err := models.RecordIDString(d.Vault)
	if err != nil {
		slog.Warn("unexpected document vault ID format in GraphQL conversion", "path", d.Path, "error", err)
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
		slog.Warn("unexpected wiki-link ID format", "raw_target", l.RawTarget, "error", err)
		id = fmt.Sprintf("%v", l.ID.ID)
	}
	fromDocID, err := models.RecordIDString(l.FromDoc)
	if err != nil {
		slog.Warn("unexpected wiki-link fromDoc ID format", "raw_target", l.RawTarget, "error", err)
		fromDocID = fmt.Sprintf("%v", l.FromDoc.ID)
	}
	var toDocID *string
	if l.ToDoc != nil {
		s, err := models.RecordIDString(*l.ToDoc)
		if err != nil {
			slog.Warn("unexpected wiki-link toDoc ID format", "raw_target", l.RawTarget, "error", err)
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
		slog.Warn("unexpected relation ID format", "rel_type", r.RelType, "error", err)
		id = fmt.Sprintf("%v", r.ID.ID)
	}
	inID, err := models.RecordIDString(r.In)
	if err != nil {
		slog.Warn("unexpected relation In ID format", "rel_type", r.RelType, "error", err)
		inID = fmt.Sprintf("%v", r.In.ID)
	}
	outID, err := models.RecordIDString(r.Out)
	if err != nil {
		slog.Warn("unexpected relation Out ID format", "rel_type", r.RelType, "error", err)
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
		slog.Warn("unexpected template ID format", "name", t.Name, "error", err)
		id = fmt.Sprintf("%v", t.ID.ID)
	}
	var vaultID *string
	if t.Vault != nil {
		s, err := models.RecordIDString(*t.Vault)
		if err != nil {
			slog.Warn("unexpected template vault ID format", "name", t.Name, "error", err)
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
		slog.Warn("unexpected user ID format", "name", u.Name, "error", err)
		id = fmt.Sprintf("%v", u.ID.ID)
	}
	return &User{
		ID:        id,
		Name:      u.Name,
		Email:     u.Email,
		CreatedAt: u.CreatedAt,
	}
}

func queryFormatToGraphQL(f parser.QueryFormat) QueryFormat {
	if f == parser.FormatTable {
		return QueryFormatTable
	}
	return QueryFormatList
}

// resolveQueryBlock executes a parsed query block against the database.
func resolveQueryBlock(ctx context.Context, dbClient *v2db.Client, vaultID string, parsed parser.QueryBlock) QueryBlock {
	block := QueryBlock{
		Index:    parsed.Index,
		RawQuery: parsed.RawQuery,
		Format:   queryFormatToGraphQL(parsed.Format),
		Results:  []QueryResult{},
	}

	if parsed.Error != "" {
		block.Error = &parsed.Error
		return block
	}

	// Build filter from parsed DSL
	filter := v2db.ListDocumentsFilter{
		VaultID: vaultID,
		Folder:  parsed.Folder,
		Limit:   parsed.Limit,
	}

	// Map WHERE conditions to filter fields
	for _, cond := range parsed.Conditions {
		switch {
		case cond.Field == "labels" && cond.Op == parser.OpContain:
			filter.Labels = append(filter.Labels, cond.Value)
		case cond.Field == "type" && cond.Op == parser.OpEqual:
			filter.DocType = &cond.Value
		}
	}

	docs, err := dbClient.ListDocuments(ctx, filter)
	if err != nil {
		errMsg := fmt.Sprintf("query error: %v", err)
		block.Error = &errMsg
		return block
	}

	// Post-filter for conditions the DB filter doesn't support (title CONTAINS)
	var titleContains string
	for _, cond := range parsed.Conditions {
		if cond.Field == "title" && cond.Op == parser.OpContains {
			titleContains = strings.ToLower(cond.Value)
		}
	}

	for _, doc := range docs {
		if titleContains != "" && !strings.Contains(strings.ToLower(doc.Title), titleContains) {
			continue
		}
		docID, err := models.RecordIDString(doc.ID)
		if err != nil {
			slog.Warn("failed to extract doc ID in query block resolution", "path", doc.Path, "error", err)
			docID = fmt.Sprintf("%v", doc.ID.ID)
		}
		result := QueryResult{
			DocID: docID,
			Title: doc.Title,
			Path:  doc.Path,
		}
		// Build fields map for SHOW columns
		if len(parsed.ShowFields) > 0 {
			fields := make(map[string]any)
			for _, f := range parsed.ShowFields {
				switch f {
				case "title":
					fields["title"] = doc.Title
				case "path":
					fields["path"] = doc.Path
				case "labels":
					fields["labels"] = doc.Labels
				case "doc_type":
					fields["doc_type"] = doc.DocType
				case "created_at":
					fields["created_at"] = doc.CreatedAt
				case "updated_at":
					fields["updated_at"] = doc.UpdatedAt
				case "source":
					fields["source"] = doc.Source
				}
			}
			result.Fields = fields
		}
		block.Results = append(block.Results, result)
	}

	return block
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
