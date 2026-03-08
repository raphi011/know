package graph

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/raphi011/knowhow/internal/db"
	"github.com/raphi011/knowhow/internal/diff"
	"github.com/raphi011/knowhow/internal/models"
	"github.com/raphi011/knowhow/internal/parser"
	"github.com/raphi011/knowhow/internal/search"
	"github.com/raphi011/knowhow/internal/tools"
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
	id, err := models.RecordIDString(f.ID)
	if err != nil {
		slog.Warn("unexpected folder ID format in GraphQL conversion", "path", f.Path, "error", err)
		id = fmt.Sprintf("%v", f.ID.ID)
	}
	return Folder{
		ID:        id,
		Path:      f.Path,
		Name:      f.Name,
		CreatedAt: f.CreatedAt,
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
		Source:    r.Source,
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
func resolveQueryBlock(ctx context.Context, dbClient *db.Client, vaultID string, parsed parser.QueryBlock) QueryBlock {
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
	filter := db.ListDocumentsFilter{
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

func versionToGraphQL(v *models.DocumentVersion) *DocumentVersion {
	if v == nil {
		return nil
	}
	id, err := models.RecordIDString(v.ID)
	if err != nil {
		slog.Warn("unexpected version ID format", "version", v.Version, "error", err)
		id = fmt.Sprintf("%v", v.ID.ID)
	}
	docID, err := models.RecordIDString(v.Document)
	if err != nil {
		slog.Warn("unexpected version document ID format", "version", v.Version, "error", err)
		docID = fmt.Sprintf("%v", v.Document.ID)
	}
	vaultID, err := models.RecordIDString(v.Vault)
	if err != nil {
		slog.Warn("unexpected version vault ID format", "version", v.Version, "error", err)
		vaultID = fmt.Sprintf("%v", v.Vault.ID)
	}
	return &DocumentVersion{
		ID:          id,
		DocumentID:  docID,
		VaultID:     vaultID,
		Version:     v.Version,
		Title:       v.Title,
		ContentHash: v.ContentHash,
		Source:      string(v.Source),
		CreatedAt:   v.CreatedAt,
	}
}

func hunkToGraphQL(h diff.Hunk) *DiffHunk {
	lines := make([]*DiffLine, len(h.Lines))
	for i, l := range h.Lines {
		lines[i] = diffLineToGraphQL(l)
	}
	return &DiffHunk{
		Index:    h.Index,
		OldStart: h.OldStart,
		OldLines: h.OldLines,
		NewStart: h.NewStart,
		NewLines: h.NewLines,
		Header:   h.Header(),
		Lines:    lines,
	}
}

func diffLineToGraphQL(l diff.DiffLine) *DiffLine {
	var t DiffLineTypeEnum
	switch l.Type {
	case diff.DiffAdd:
		t = DiffLineTypeAdd
	case diff.DiffDelete:
		t = DiffLineTypeDelete
	default:
		t = DiffLineTypeContext
	}
	return &DiffLine{
		Type:      t,
		Content:   l.Content,
		OldLineNo: l.OldLineNo,
		NewLineNo: l.NewLineNo,
	}
}

func conversationToGraphQL(c *models.Conversation) *Conversation {
	if c == nil {
		return nil
	}
	id, err := models.RecordIDString(c.ID)
	if err != nil {
		slog.Warn("unexpected conversation ID format", "error", err)
		id = fmt.Sprintf("%v", c.ID.ID)
	}
	vaultID, err := models.RecordIDString(c.Vault)
	if err != nil {
		slog.Warn("unexpected conversation vault ID format", "error", err)
		vaultID = fmt.Sprintf("%v", c.Vault.ID)
	}
	return &Conversation{
		ID:        id,
		VaultID:   vaultID,
		Title:     c.Title,
		CreatedAt: c.CreatedAt,
		UpdatedAt: c.UpdatedAt,
	}
}

func messageToGraphQL(m *models.Message) *ChatMessage {
	if m == nil {
		return nil
	}
	id, err := models.RecordIDString(m.ID)
	if err != nil {
		slog.Warn("unexpected message ID format", "error", err)
		id = fmt.Sprintf("%v", m.ID.ID)
	}
	docRefs := m.DocRefs
	if docRefs == nil {
		docRefs = []string{}
	}
	return &ChatMessage{
		ID:         id,
		Role:       string(m.Role),
		Content:    m.Content,
		DocRefs:    docRefs,
		ToolName:   m.ToolName,
		ToolInput:  m.ToolInput,
		ToolMeta:   toolResultMetaFromJSON(m.ToolMeta),
		ToolCallID: m.ToolCallID,
		ToolCalls:  m.ToolCalls,
		CreatedAt:  m.CreatedAt,
	}
}

func toolResultMetaFromJSON(s *string) *ToolResultMeta {
	if s == nil || *s == "" {
		return nil
	}
	var src tools.ToolResultMeta
	if err := json.Unmarshal([]byte(*s), &src); err != nil {
		slog.Warn("failed to unmarshal tool_meta JSON", "error", err, "raw", *s)
		return nil
	}
	meta := &ToolResultMeta{
		DurationMs:     int(src.DurationMs),
		ResultCount:    src.ResultCount,
		ChunkCount:     src.ChunkCount,
		DocumentPath:   src.DocumentPath,
		DocumentTitle:  src.DocumentTitle,
		ContentLength:  src.ContentLength,
		WebResultCount: src.WebResultCount,
	}
	for _, d := range src.MatchedDocs {
		meta.MatchedDocs = append(meta.MatchedDocs, &ToolDocRef{
			Title: d.Title,
			Path:  d.Path,
			Score: d.Score,
		})
	}
	for _, w := range src.WebSources {
		meta.WebSources = append(meta.WebSources, &ToolWebRef{
			Title: w.Title,
			URL:   w.URL,
		})
	}
	return meta
}

func vaultMemberToGraphQL(dbClient *db.Client, ctx context.Context, m *models.VaultMember) (*VaultMember, error) {
	userID, err := models.RecordIDString(m.User)
	if err != nil {
		return nil, fmt.Errorf("extract user ID: %w", err)
	}
	vaultID, err := models.RecordIDString(m.Vault)
	if err != nil {
		return nil, fmt.Errorf("extract vault ID: %w", err)
	}
	// Fetch user name
	user, err := dbClient.GetUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}
	userName := userID
	if user != nil {
		userName = user.Name
	}
	return &VaultMember{
		UserID:    userID,
		UserName:  userName,
		VaultID:   vaultID,
		Role:      m.Role,
		CreatedAt: m.CreatedAt,
	}, nil
}

func shareLinkToGraphQL(l *models.ShareLink) *ShareLink {
	if l == nil {
		return nil
	}
	id, err := models.RecordIDString(l.ID)
	if err != nil {
		slog.Warn("unexpected share link ID format", "error", err)
		id = fmt.Sprintf("%v", l.ID.ID)
	}
	vaultID, err := models.RecordIDString(l.Vault)
	if err != nil {
		slog.Warn("unexpected share link vault ID format", "error", err)
		vaultID = fmt.Sprintf("%v", l.Vault.ID)
	}
	return &ShareLink{
		ID:        id,
		VaultID:   vaultID,
		Path:      l.Path,
		IsFolder:  l.IsFolder,
		ExpiresAt: l.ExpiresAt,
		CreatedAt: l.CreatedAt,
	}
}

func syncMetaToGraphQL(m db.SyncMeta) *SyncMeta {
	id, err := models.RecordIDString(m.ID)
	if err != nil {
		slog.Warn("unexpected sync meta ID format", "path", m.Path, "error", err)
		id = fmt.Sprintf("%v", m.ID.ID)
	}
	return &SyncMeta{
		ID:          id,
		Path:        m.Path,
		ContentHash: m.ContentHash,
		UpdatedAt:   m.UpdatedAt,
	}
}

func tombstoneToGraphQL(t db.SyncTombstone) *SyncTombstone {
	return &SyncTombstone{
		DocID:     t.DocID,
		Path:      t.Path,
		DeletedAt: t.DeletedAt,
	}
}

func searchResultToGraphQL(r search.SearchResult) SearchResult {
	chunks := make([]ChunkMatch, len(r.MatchedChunks))
	for i, ch := range r.MatchedChunks {
		chunks[i] = ChunkMatch{
			Snippet:     ch.Snippet,
			HeadingPath: ch.HeadingPath,
			Position:    ch.Position,
			Score:       ch.Score,
		}
	}
	return SearchResult{
		DocumentID:    r.DocumentID,
		Path:          r.Path,
		Title:         r.Title,
		Labels:        r.Labels,
		DocType:       r.DocType,
		Score:         r.Score,
		MatchedChunks: chunks,
	}
}
