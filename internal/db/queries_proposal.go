package db

import (
	"context"
	"fmt"

	"github.com/raphaelgruber/memcp-go/internal/models"
	"github.com/surrealdb/surrealdb.go"
)

func (c *Client) CreateProposal(ctx context.Context, input models.DocumentProposalInput) (*models.DocumentProposal, error) {
	start := c.startOp()
	defer c.recordTiming("db.create_proposal", start)

	sql := `
		CREATE document_proposal SET
			vault = type::record("vault", $vault_id),
			document = type::record("document", $document_id),
			proposed_content = $proposed_content,
			description = $description,
			source = $source,
			original_hash = $original_hash
		RETURN AFTER
	`
	results, err := surrealdb.Query[[]models.DocumentProposal](ctx, c.DB(), sql, map[string]any{
		"vault_id":         input.VaultID,
		"document_id":      input.DocumentID,
		"proposed_content": input.ProposedContent,
		"description":      optionalString(input.Description),
		"source":           string(input.Source),
		"original_hash":    input.OriginalHash,
	})
	if err != nil {
		return nil, fmt.Errorf("create proposal: %w", err)
	}
	if results == nil || len(*results) == 0 || len((*results)[0].Result) == 0 {
		return nil, fmt.Errorf("create proposal: no result returned")
	}
	return &(*results)[0].Result[0], nil
}

func (c *Client) GetProposal(ctx context.Context, id string) (*models.DocumentProposal, error) {
	start := c.startOp()
	defer c.recordTiming("db.get_proposal", start)

	sql := `SELECT * FROM type::record("document_proposal", $id)`
	results, err := surrealdb.Query[[]models.DocumentProposal](ctx, c.DB(), sql, map[string]any{
		"id": id,
	})
	if err != nil {
		return nil, fmt.Errorf("get proposal: %w", err)
	}
	if results == nil || len(*results) == 0 || len((*results)[0].Result) == 0 {
		return nil, nil
	}
	return &(*results)[0].Result[0], nil
}

func (c *Client) ListProposals(ctx context.Context, vaultID string, status *string) ([]models.DocumentProposal, error) {
	start := c.startOp()
	defer c.recordTiming("db.list_proposals", start)

	var sql string
	vars := map[string]any{
		"vault_id": vaultID,
	}

	if status != nil {
		sql = `SELECT * FROM document_proposal WHERE vault = type::record("vault", $vault_id) AND status = $status ORDER BY created_at DESC`
		vars["status"] = *status
	} else {
		sql = `SELECT * FROM document_proposal WHERE vault = type::record("vault", $vault_id) ORDER BY created_at DESC`
	}

	results, err := surrealdb.Query[[]models.DocumentProposal](ctx, c.DB(), sql, vars)
	if err != nil {
		return nil, fmt.Errorf("list proposals: %w", err)
	}
	if results == nil || len(*results) == 0 {
		return nil, nil
	}
	return (*results)[0].Result, nil
}

func (c *Client) ListProposalsByDocument(ctx context.Context, documentID string, status *string) ([]models.DocumentProposal, error) {
	start := c.startOp()
	defer c.recordTiming("db.list_proposals_by_document", start)

	var sql string
	vars := map[string]any{
		"document_id": documentID,
	}

	if status != nil {
		sql = `SELECT * FROM document_proposal WHERE document = type::record("document", $document_id) AND status = $status ORDER BY created_at DESC`
		vars["status"] = *status
	} else {
		sql = `SELECT * FROM document_proposal WHERE document = type::record("document", $document_id) ORDER BY created_at DESC`
	}

	results, err := surrealdb.Query[[]models.DocumentProposal](ctx, c.DB(), sql, vars)
	if err != nil {
		return nil, fmt.Errorf("list proposals by document: %w", err)
	}
	if results == nil || len(*results) == 0 {
		return nil, nil
	}
	return (*results)[0].Result, nil
}

func (c *Client) UpdateProposalStatus(ctx context.Context, id string, status models.ProposalStatus, notes *string) error {
	start := c.startOp()
	defer c.recordTiming("db.update_proposal_status", start)

	sql := `
		UPDATE type::record("document_proposal", $id) SET
			status = $status,
			reviewer_notes = $notes,
			reviewed_at = time::now()
		RETURN AFTER
	`
	results, err := surrealdb.Query[[]models.DocumentProposal](ctx, c.DB(), sql, map[string]any{
		"id":     id,
		"status": string(status),
		"notes":  optionalString(notes),
	})
	if err != nil {
		return fmt.Errorf("update proposal status: %w", err)
	}
	if results == nil || len(*results) == 0 || len((*results)[0].Result) == 0 {
		return fmt.Errorf("update proposal status: proposal not found: %s", id)
	}
	return nil
}

