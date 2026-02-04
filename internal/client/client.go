// Package client provides a GraphQL client for the Knowhow server.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// Client is a GraphQL client for the Knowhow server.
type Client struct {
	endpoint   string
	httpClient *http.Client
}

// New creates a new GraphQL client.
// If endpoint is empty, uses KNOWHOW_SERVER_URL env var or defaults to localhost:8080.
func New(endpoint string) *Client {
	if endpoint == "" {
		endpoint = os.Getenv("KNOWHOW_SERVER_URL")
	}
	if endpoint == "" {
		endpoint = "http://localhost:8080/query"
	}

	return &Client{
		endpoint: endpoint,
		httpClient: &http.Client{
			Timeout: 120 * time.Second, // Long timeout for LLM operations
		},
	}
}

// graphQLRequest is the request payload for GraphQL operations.
type graphQLRequest struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables,omitempty"`
}

// graphQLResponse is the response payload from GraphQL operations.
type graphQLResponse struct {
	Data   json.RawMessage `json:"data"`
	Errors []graphQLError  `json:"errors,omitempty"`
}

// graphQLError represents a GraphQL error.
type graphQLError struct {
	Message string `json:"message"`
	Path    []any  `json:"path,omitempty"`
}

// Execute sends a GraphQL query/mutation and returns the result.
func (c *Client) Execute(ctx context.Context, query string, variables map[string]any, result any) error {
	reqBody, err := json.Marshal(graphQLRequest{
		Query:     query,
		Variables: variables,
	})
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.endpoint, bytes.NewReader(reqBody))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server error: %s - %s", resp.Status, string(body))
	}

	var gqlResp graphQLResponse
	if err := json.Unmarshal(body, &gqlResp); err != nil {
		return fmt.Errorf("unmarshal response: %w", err)
	}

	if len(gqlResp.Errors) > 0 {
		return fmt.Errorf("graphql error: %s", gqlResp.Errors[0].Message)
	}

	if result != nil && len(gqlResp.Data) > 0 {
		if err := json.Unmarshal(gqlResp.Data, result); err != nil {
			return fmt.Errorf("unmarshal data: %w", err)
		}
	}

	return nil
}

// =============================================================================
// TYPES (matching GraphQL schema)
// =============================================================================

// Entity represents a knowledge entity.
type Entity struct {
	ID          string         `json:"id"`
	Type        string         `json:"type"`
	Name        string         `json:"name"`
	Content     *string        `json:"content,omitempty"`
	Summary     *string        `json:"summary,omitempty"`
	Labels      []string       `json:"labels"`
	Verified    bool           `json:"verified"`
	Confidence  float64        `json:"confidence"`
	Source      string         `json:"source"`
	SourcePath  *string        `json:"sourcePath,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	CreatedAt   time.Time      `json:"createdAt"`
	UpdatedAt   time.Time      `json:"updatedAt"`
	AccessedAt  time.Time      `json:"accessedAt"`
	AccessCount int            `json:"accessCount"`
}

// Template represents an output rendering template.
type Template struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description *string   `json:"description,omitempty"`
	Content     string    `json:"content"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// EntitySearchResult wraps search results with match context.
type EntitySearchResult struct {
	Entity        Entity       `json:"entity"`
	MatchedChunks []ChunkMatch `json:"matchedChunks"`
	Score         float64      `json:"score"`
}

// ChunkMatch represents a matching chunk within a search result.
type ChunkMatch struct {
	Content     string  `json:"content"`
	HeadingPath *string `json:"headingPath,omitempty"`
	Position    int     `json:"position"`
}

// IngestResult summarizes an ingestion operation.
type IngestResult struct {
	FilesProcessed   int      `json:"filesProcessed"`
	EntitiesCreated  int      `json:"entitiesCreated"`
	ChunksCreated    int      `json:"chunksCreated"`
	RelationsCreated int      `json:"relationsCreated"`
	Errors           []string `json:"errors"`
}

// LabelCount represents a label with its entity count.
type LabelCount struct {
	Label string `json:"label"`
	Count int    `json:"count"`
}

// TypeCount represents an entity type with its count.
type TypeCount struct {
	Type  string `json:"type"`
	Count int    `json:"count"`
}

// TokenUsageSummary provides aggregated token usage statistics.
type TokenUsageSummary struct {
	TotalTokens  int            `json:"totalTokens"`
	TotalCostUSD float64        `json:"totalCostUSD"`
	ByOperation  map[string]any `json:"byOperation"`
	ByModel      map[string]any `json:"byModel"`
}

// =============================================================================
// ENTITY OPERATIONS
// =============================================================================

// CreateEntityInput is the input for creating an entity.
type CreateEntityInput struct {
	Type       string         `json:"type"`
	Name       string         `json:"name"`
	Content    *string        `json:"content,omitempty"`
	Summary    *string        `json:"summary,omitempty"`
	Labels     []string       `json:"labels,omitempty"`
	Verified   *bool          `json:"verified,omitempty"`
	Source     *string        `json:"source,omitempty"`
	SourcePath *string        `json:"sourcePath,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

// CreateEntity creates a new entity.
func (c *Client) CreateEntity(ctx context.Context, input CreateEntityInput) (*Entity, error) {
	const query = `
		mutation CreateEntity($input: EntityInput!) {
			createEntity(input: $input) {
				id type name content summary labels verified confidence
				source sourcePath metadata createdAt updatedAt accessedAt accessCount
			}
		}
	`

	var result struct {
		CreateEntity Entity `json:"createEntity"`
	}
	if err := c.Execute(ctx, query, map[string]any{"input": input}, &result); err != nil {
		return nil, err
	}
	return &result.CreateEntity, nil
}

// UpdateEntityInput is the input for updating an entity.
type UpdateEntityInput struct {
	Name      *string        `json:"name,omitempty"`
	Content   *string        `json:"content,omitempty"`
	Summary   *string        `json:"summary,omitempty"`
	Labels    []string       `json:"labels,omitempty"`
	AddLabels []string       `json:"addLabels,omitempty"`
	DelLabels []string       `json:"delLabels,omitempty"`
	Verified  *bool          `json:"verified,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

// UpdateEntity updates an existing entity.
func (c *Client) UpdateEntity(ctx context.Context, id string, input UpdateEntityInput) (*Entity, error) {
	const query = `
		mutation UpdateEntity($id: ID!, $input: EntityUpdate!) {
			updateEntity(id: $id, input: $input) {
				id type name content summary labels verified confidence
				source sourcePath metadata createdAt updatedAt accessedAt accessCount
			}
		}
	`

	var result struct {
		UpdateEntity Entity `json:"updateEntity"`
	}
	if err := c.Execute(ctx, query, map[string]any{"id": id, "input": input}, &result); err != nil {
		return nil, err
	}
	return &result.UpdateEntity, nil
}

// DeleteEntity deletes an entity by ID.
func (c *Client) DeleteEntity(ctx context.Context, id string) (bool, error) {
	const query = `
		mutation DeleteEntity($id: ID!) {
			deleteEntity(id: $id)
		}
	`

	var result struct {
		DeleteEntity bool `json:"deleteEntity"`
	}
	if err := c.Execute(ctx, query, map[string]any{"id": id}, &result); err != nil {
		return false, err
	}
	return result.DeleteEntity, nil
}

// GetEntity retrieves an entity by ID.
func (c *Client) GetEntity(ctx context.Context, id string) (*Entity, error) {
	const query = `
		query GetEntity($id: ID!) {
			entity(id: $id) {
				id type name content summary labels verified confidence
				source sourcePath metadata createdAt updatedAt accessedAt accessCount
			}
		}
	`

	var result struct {
		Entity *Entity `json:"entity"`
	}
	if err := c.Execute(ctx, query, map[string]any{"id": id}, &result); err != nil {
		return nil, err
	}
	return result.Entity, nil
}

// GetEntityByName retrieves an entity by name.
func (c *Client) GetEntityByName(ctx context.Context, name string) (*Entity, error) {
	const query = `
		query GetEntityByName($name: String!) {
			entityByName(name: $name) {
				id type name content summary labels verified confidence
				source sourcePath metadata createdAt updatedAt accessedAt accessCount
			}
		}
	`

	var result struct {
		EntityByName *Entity `json:"entityByName"`
	}
	if err := c.Execute(ctx, query, map[string]any{"name": name}, &result); err != nil {
		return nil, err
	}
	return result.EntityByName, nil
}

// ListEntitiesOptions configures entity listing.
type ListEntitiesOptions struct {
	Type   *string
	Labels []string
	Limit  *int
}

// ListEntities returns entities with optional filtering.
func (c *Client) ListEntities(ctx context.Context, opts ListEntitiesOptions) ([]Entity, error) {
	const query = `
		query ListEntities($type: String, $labels: [String!], $limit: Int) {
			entities(type: $type, labels: $labels, limit: $limit) {
				id type name content summary labels verified confidence
				source sourcePath metadata createdAt updatedAt accessedAt accessCount
			}
		}
	`

	vars := map[string]any{}
	if opts.Type != nil {
		vars["type"] = *opts.Type
	}
	if len(opts.Labels) > 0 {
		vars["labels"] = opts.Labels
	}
	if opts.Limit != nil {
		vars["limit"] = *opts.Limit
	}

	var result struct {
		Entities []Entity `json:"entities"`
	}
	if err := c.Execute(ctx, query, vars, &result); err != nil {
		return nil, err
	}
	return result.Entities, nil
}

// =============================================================================
// SEARCH OPERATIONS
// =============================================================================

// SearchOptions configures search operations.
type SearchOptions struct {
	Query        string
	Labels       []string
	Types        []string
	VerifiedOnly *bool
	Limit        *int
}

// Search performs hybrid search.
func (c *Client) Search(ctx context.Context, opts SearchOptions) ([]EntitySearchResult, error) {
	const query = `
		query Search($input: SearchInput!) {
			search(input: $input) {
				entity {
					id type name content summary labels verified confidence
					source sourcePath metadata createdAt updatedAt accessedAt accessCount
				}
				matchedChunks { content headingPath position }
				score
			}
		}
	`

	input := map[string]any{"query": opts.Query}
	if len(opts.Labels) > 0 {
		input["labels"] = opts.Labels
	}
	if len(opts.Types) > 0 {
		input["types"] = opts.Types
	}
	if opts.VerifiedOnly != nil {
		input["verifiedOnly"] = *opts.VerifiedOnly
	}
	if opts.Limit != nil {
		input["limit"] = *opts.Limit
	}

	var result struct {
		Search []EntitySearchResult `json:"search"`
	}
	if err := c.Execute(ctx, query, map[string]any{"input": input}, &result); err != nil {
		return nil, err
	}
	return result.Search, nil
}

// Ask performs search and synthesizes an answer using LLM.
func (c *Client) Ask(ctx context.Context, question string, opts *SearchOptions, templateName *string) (string, error) {
	const query = `
		query Ask($query: String!, $input: SearchInput, $templateName: String) {
			ask(query: $query, input: $input, templateName: $templateName)
		}
	`

	vars := map[string]any{"query": question}
	if opts != nil {
		input := map[string]any{}
		if opts.Query != "" {
			input["query"] = opts.Query
		} else {
			input["query"] = question
		}
		if len(opts.Labels) > 0 {
			input["labels"] = opts.Labels
		}
		if len(opts.Types) > 0 {
			input["types"] = opts.Types
		}
		if opts.VerifiedOnly != nil {
			input["verifiedOnly"] = *opts.VerifiedOnly
		}
		if opts.Limit != nil {
			input["limit"] = *opts.Limit
		}
		vars["input"] = input
	}
	if templateName != nil {
		vars["templateName"] = *templateName
	}

	var result struct {
		Ask string `json:"ask"`
	}
	if err := c.Execute(ctx, query, vars, &result); err != nil {
		return "", err
	}
	return result.Ask, nil
}

// =============================================================================
// RELATION OPERATIONS
// =============================================================================

// CreateRelationInput is the input for creating a relation.
type CreateRelationInput struct {
	FromID   string   `json:"fromId"`
	ToID     string   `json:"toId"`
	RelType  string   `json:"relType"`
	Strength *float64 `json:"strength,omitempty"`
}

// CreateRelation creates a relation between entities.
func (c *Client) CreateRelation(ctx context.Context, input CreateRelationInput) (bool, error) {
	const query = `
		mutation CreateRelation($input: RelationInput!) {
			createRelation(input: $input)
		}
	`

	var result struct {
		CreateRelation bool `json:"createRelation"`
	}
	if err := c.Execute(ctx, query, map[string]any{"input": input}, &result); err != nil {
		return false, err
	}
	return result.CreateRelation, nil
}

// =============================================================================
// INGEST OPERATIONS
// =============================================================================

// IngestOptions configures ingestion.
type IngestOptions struct {
	Labels       []string
	ExtractGraph *bool
	DryRun       *bool
	Recursive    *bool
}

// IngestFile ingests a single file.
func (c *Client) IngestFile(ctx context.Context, filePath string, opts *IngestOptions) (*Entity, error) {
	const query = `
		mutation IngestFile($filePath: String!, $input: IngestInput) {
			ingestFile(filePath: $filePath, input: $input) {
				id type name content summary labels verified confidence
				source sourcePath metadata createdAt updatedAt accessedAt accessCount
			}
		}
	`

	vars := map[string]any{"filePath": filePath}
	if opts != nil {
		input := map[string]any{}
		if len(opts.Labels) > 0 {
			input["labels"] = opts.Labels
		}
		if opts.ExtractGraph != nil {
			input["extractGraph"] = *opts.ExtractGraph
		}
		if opts.DryRun != nil {
			input["dryRun"] = *opts.DryRun
		}
		if opts.Recursive != nil {
			input["recursive"] = *opts.Recursive
		}
		vars["input"] = input
	}

	var result struct {
		IngestFile Entity `json:"ingestFile"`
	}
	if err := c.Execute(ctx, query, vars, &result); err != nil {
		return nil, err
	}
	return &result.IngestFile, nil
}

// IngestDirectory ingests all files from a directory.
func (c *Client) IngestDirectory(ctx context.Context, dirPath string, opts *IngestOptions) (*IngestResult, error) {
	const query = `
		mutation IngestDirectory($dirPath: String!, $input: IngestInput) {
			ingestDirectory(dirPath: $dirPath, input: $input) {
				filesProcessed entitiesCreated chunksCreated relationsCreated errors
			}
		}
	`

	vars := map[string]any{"dirPath": dirPath}
	if opts != nil {
		input := map[string]any{}
		if len(opts.Labels) > 0 {
			input["labels"] = opts.Labels
		}
		if opts.ExtractGraph != nil {
			input["extractGraph"] = *opts.ExtractGraph
		}
		if opts.DryRun != nil {
			input["dryRun"] = *opts.DryRun
		}
		if opts.Recursive != nil {
			input["recursive"] = *opts.Recursive
		}
		vars["input"] = input
	}

	var result struct {
		IngestDirectory IngestResult `json:"ingestDirectory"`
	}
	if err := c.Execute(ctx, query, vars, &result); err != nil {
		return nil, err
	}
	return &result.IngestDirectory, nil
}

// =============================================================================
// TEMPLATE OPERATIONS
// =============================================================================

// CreateTemplate creates a new template.
func (c *Client) CreateTemplate(ctx context.Context, name string, description *string, content string) (*Template, error) {
	const query = `
		mutation CreateTemplate($name: String!, $description: String, $content: String!) {
			createTemplate(name: $name, description: $description, content: $content) {
				id name description content createdAt updatedAt
			}
		}
	`

	vars := map[string]any{"name": name, "content": content}
	if description != nil {
		vars["description"] = *description
	}

	var result struct {
		CreateTemplate Template `json:"createTemplate"`
	}
	if err := c.Execute(ctx, query, vars, &result); err != nil {
		return nil, err
	}
	return &result.CreateTemplate, nil
}

// DeleteTemplate deletes a template by name.
func (c *Client) DeleteTemplate(ctx context.Context, name string) (bool, error) {
	const query = `
		mutation DeleteTemplate($name: String!) {
			deleteTemplate(name: $name)
		}
	`

	var result struct {
		DeleteTemplate bool `json:"deleteTemplate"`
	}
	if err := c.Execute(ctx, query, map[string]any{"name": name}, &result); err != nil {
		return false, err
	}
	return result.DeleteTemplate, nil
}

// GetTemplate retrieves a template by name.
func (c *Client) GetTemplate(ctx context.Context, name string) (*Template, error) {
	const query = `
		query GetTemplate($name: String!) {
			template(name: $name) {
				id name description content createdAt updatedAt
			}
		}
	`

	var result struct {
		Template *Template `json:"template"`
	}
	if err := c.Execute(ctx, query, map[string]any{"name": name}, &result); err != nil {
		return nil, err
	}
	return result.Template, nil
}

// ListTemplates returns all templates.
func (c *Client) ListTemplates(ctx context.Context) ([]Template, error) {
	const query = `
		query ListTemplates {
			templates {
				id name description content createdAt updatedAt
			}
		}
	`

	var result struct {
		Templates []Template `json:"templates"`
	}
	if err := c.Execute(ctx, query, nil, &result); err != nil {
		return nil, err
	}
	return result.Templates, nil
}

// =============================================================================
// LIST OPERATIONS
// =============================================================================

// ListLabels returns unique labels with entity counts.
func (c *Client) ListLabels(ctx context.Context) ([]LabelCount, error) {
	const query = `
		query ListLabels {
			labels { label count }
		}
	`

	var result struct {
		Labels []LabelCount `json:"labels"`
	}
	if err := c.Execute(ctx, query, nil, &result); err != nil {
		return nil, err
	}
	return result.Labels, nil
}

// ListTypes returns entity types with counts.
func (c *Client) ListTypes(ctx context.Context) ([]TypeCount, error) {
	const query = `
		query ListTypes {
			types { type count }
		}
	`

	var result struct {
		Types []TypeCount `json:"types"`
	}
	if err := c.Execute(ctx, query, nil, &result); err != nil {
		return nil, err
	}
	return result.Types, nil
}

// =============================================================================
// USAGE OPERATIONS
// =============================================================================

// GetUsageSummary returns token usage statistics.
func (c *Client) GetUsageSummary(ctx context.Context, since string) (*TokenUsageSummary, error) {
	const query = `
		query GetUsageSummary($since: String!) {
			usageSummary(since: $since) {
				totalTokens totalCostUSD byOperation byModel
			}
		}
	`

	var result struct {
		UsageSummary TokenUsageSummary `json:"usageSummary"`
	}
	if err := c.Execute(ctx, query, map[string]any{"since": since}, &result); err != nil {
		return nil, err
	}
	return &result.UsageSummary, nil
}
