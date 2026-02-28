# Design: Agent Document Approval System

## Overview

Enable AI agents to propose updates to existing documents, with a human-in-the-loop review system that lets users approve or reject individual diff hunks (like `git add -p`) or accept/reject entire changes at once.

The **GraphQL API** is the single entry point for both proposing and reviewing changes. The review UI lives in a separate web project that consumes this API.

---

## Data Model

### SurrealDB Table: `document_proposal`

```surql
DEFINE TABLE IF NOT EXISTS document_proposal SCHEMAFULL;

DEFINE FIELD IF NOT EXISTS vault        ON document_proposal TYPE record<vault>;
DEFINE FIELD IF NOT EXISTS document     ON document_proposal TYPE record<document>;
DEFINE FIELD IF NOT EXISTS proposed_content ON document_proposal TYPE string;
DEFINE FIELD IF NOT EXISTS description  ON document_proposal TYPE option<string>;
DEFINE FIELD IF NOT EXISTS source       ON document_proposal TYPE string DEFAULT "ai_suggested";
DEFINE FIELD IF NOT EXISTS status       ON document_proposal TYPE string DEFAULT "pending";
DEFINE FIELD IF NOT EXISTS original_hash ON document_proposal TYPE string;
    -- content_hash of document at proposal time — for conflict detection
DEFINE FIELD IF NOT EXISTS reviewed_at  ON document_proposal TYPE option<datetime>;
DEFINE FIELD IF NOT EXISTS reviewer_notes ON document_proposal TYPE option<string>;
DEFINE FIELD IF NOT EXISTS created_at   ON document_proposal TYPE datetime DEFAULT time::now();

DEFINE INDEX IF NOT EXISTS idx_proposal_vault    ON document_proposal FIELDS vault;
DEFINE INDEX IF NOT EXISTS idx_proposal_document ON document_proposal FIELDS document;
DEFINE INDEX IF NOT EXISTS idx_proposal_status   ON document_proposal FIELDS status;

-- Cascade: delete proposals when document is deleted
DEFINE EVENT IF NOT EXISTS cascade_delete_document_proposals ON document
WHEN $event = "DELETE" THEN {
    DELETE FROM document_proposal WHERE document = $before.id
};
```

### Go Model: `models/proposal.go`

```go
package models

import (
    "time"
    surrealmodels "github.com/surrealdb/surrealdb.go/pkg/models"
)

type ProposalStatus string

const (
    ProposalPending            ProposalStatus = "pending"
    ProposalApproved           ProposalStatus = "approved"
    ProposalPartiallyApproved  ProposalStatus = "partially_approved"
    ProposalRejected           ProposalStatus = "rejected"
    ProposalConflict           ProposalStatus = "conflict"
    ProposalExpired            ProposalStatus = "expired"
)

type ProposalSource string

const (
    ProposalSourceAISuggested ProposalSource = "ai_suggested"
    ProposalSourceAIGenerated ProposalSource = "ai_generated"
    ProposalSourceImport      ProposalSource = "import"
)

type DocumentProposal struct {
    ID              surrealmodels.RecordID `json:"id"`
    Vault           surrealmodels.RecordID `json:"vault"`
    Document        surrealmodels.RecordID `json:"document"`
    ProposedContent string                 `json:"proposed_content"`
    Description     *string                `json:"description,omitempty"`
    Source          ProposalSource         `json:"source"`
    Status          ProposalStatus         `json:"status"`
    OriginalHash    string                 `json:"original_hash"`
    ReviewedAt      *time.Time             `json:"reviewed_at,omitempty"`
    ReviewerNotes   *string                `json:"reviewer_notes,omitempty"`
    CreatedAt       time.Time              `json:"created_at"`
}

type DocumentProposalInput struct {
    VaultID         string `json:"vault_id"`
    DocumentID      string `json:"document_id"`
    ProposedContent string `json:"proposed_content"`
    Description     *string `json:"description,omitempty"`
    Source          ProposalSource `json:"source"`
    OriginalHash    string `json:"original_hash"`
}
```

---

## Diff Computation

### Library: `github.com/pmezard/go-difflib`

Direct dependency. Provides `difflib.NewMatcher` + `GetGroupedOpCodes` for structured opcode-based diff computation.

### Diff Types (computed, not stored)

```go
// internal/review/diff.go

// Hunk represents a contiguous group of changes in a unified diff.
type Hunk struct {
    Index      int        `json:"index"`
    OldStart   int        `json:"old_start"`    // starting line in original
    OldLines   int        `json:"old_lines"`    // number of lines from original
    NewStart   int        `json:"new_start"`    // starting line in proposed
    NewLines   int        `json:"new_lines"`    // number of lines from proposed
    Lines      []DiffLine `json:"lines"`
}

type DiffLineType string

const (
    DiffContext DiffLineType = "context"
    DiffAdd     DiffLineType = "add"
    DiffDelete  DiffLineType = "delete"
)

type DiffLine struct {
    Type      DiffLineType `json:"type"`
    Content   string       `json:"content"`
    OldLineNo *int         `json:"old_line_no,omitempty"`
    NewLineNo *int         `json:"new_line_no,omitempty"`
}
```

### Computing Hunks

```go
func ComputeHunks(original, proposed string, contextLines int) ([]Hunk, error)
```

1. Split both strings into lines
2. Use `difflib.SequenceMatcher` to get opcodes (matching/replacing/inserting/deleting blocks)
3. Group contiguous changes with `contextLines` surrounding context into hunks
4. Return structured `[]Hunk` for the API to serialize

### Applying Selected Hunks (Partial Approval)

```go
func ApplyHunks(original string, hunks []Hunk, acceptedIndexes []int) (string, error)
```

Algorithm:
1. Split original into lines
2. Build a set of accepted indexes for O(1) lookup. Sort all hunks by `OldStart` to process in document order
3. Walk through all hunks in order, maintaining a cursor (`origPos`) into the original lines. For each hunk:
   - Copy unchanged lines before the hunk
   - If **accepted**: emit context and added lines, skip deleted lines
   - If **rejected**: emit context and deleted lines (original), skip added lines
4. Copy remaining lines after the last hunk
5. Join lines back into string and return the merged result

This is equivalent to how `git apply --cached` works with partial patches — hunks are independent units that can be individually toggled.

**Conflict handling**: Before computing hunks, compare `proposal.OriginalHash` with the document's current `content_hash`. If they differ, the document has been modified since the proposal was created. Options:
- Mark proposal as `conflict` and require the agent to re-propose against current content
- Attempt three-way merge (future enhancement)

---

## Service Layer: `internal/review/`

```go
// internal/review/service.go

type Service struct {
    db          *db.Client
    docService  *document.Service
}

func NewService(db *db.Client, docService *document.Service) *Service

// Create stores a new document proposal, capturing the document's current content hash.
// Returns error if proposed content is identical to current document content.
func (s *Service) Create(ctx context.Context, vaultID, documentID, proposedContent string, description *string, source models.ProposalSource) (*models.DocumentProposal, error)

// CreateByPath stores a new proposal by looking up the document by vault+path.
func (s *Service) CreateByPath(ctx context.Context, vaultID, path, proposedContent string, description *string, source models.ProposalSource) (*models.DocumentProposal, error)

// Get retrieves a single proposal by ID.
func (s *Service) Get(ctx context.Context, id string) (*models.DocumentProposal, error)

// List returns proposals for a vault, optionally filtered by status.
func (s *Service) List(ctx context.Context, vaultID string, status *models.ProposalStatus) ([]models.DocumentProposal, error)

// ListForDocument returns proposals for a specific document, optionally filtered by status.
func (s *Service) ListForDocument(ctx context.Context, documentID string, status *models.ProposalStatus) ([]models.DocumentProposal, error)

// Diff computes the diff between the current document content and the proposal,
// and detects conflicts by comparing content hashes.
func (s *Service) Diff(ctx context.Context, proposal *models.DocumentProposal) (*DiffResult, error)

// DiffResult contains the computed diff for a proposal.
type DiffResult struct {
    Hunks       []Hunk
    HasConflict bool      // true if document changed since proposal
    Stats       DiffStats // additions, deletions, hunks count
}

// ApproveAll approves the entire proposal and applies it to the document.
// Returns error if proposal is not in a reviewable state or conflicts with current document.
func (s *Service) ApproveAll(ctx context.Context, proposalID string, notes *string) (*models.Document, error)

// ApproveHunks approves specific hunks, merges them, and updates the document.
// Returns error if hunkIndexes is empty, proposal is not reviewable, or conflicts exist.
func (s *Service) ApproveHunks(ctx context.Context, proposalID string, hunkIndexes []int, notes *string) (*models.Document, error)

// Reject marks a proposal as rejected.
// Returns error if proposal is not in a reviewable state.
func (s *Service) Reject(ctx context.Context, proposalID string, notes *string) error
```

### Workflow

```
Agent proposes update
        │
        ▼
  ┌─────────────┐
  │   pending    │
  └──────┬──────┘
         │
    User reviews diff
         │
    ┌────┼────────────┐
    ▼    ▼            ▼
 approve approve    reject
   all   hunks
    │      │          │
    ▼      ▼          ▼
┌────────┐ ┌──────────┐ ┌──────────┐
│approved│ │partially │ │ rejected │
│        │ │_approved │ │          │
└───┬────┘ └────┬─────┘ └──────────┘
    │           │
    └─────┬─────┘
          ▼
  documentService.Update()
  (re-parse, re-chunk, re-embed)
```

---

## GraphQL Schema Additions

```graphql
# =============================================================================
# PROPOSAL TYPES
# =============================================================================

enum ProposalStatus {
  PENDING
  APPROVED
  PARTIALLY_APPROVED
  REJECTED
  CONFLICT
  EXPIRED
}

type DocumentProposal {
  id: ID!
  vaultId: ID!
  document: Document!
  proposedContent: String!
  description: String
  source: ProposalSource!
  status: ProposalStatus!
  originalHash: String!
  hasConflict: Boolean!
  reviewedAt: DateTime
  reviewerNotes: String
  createdAt: DateTime!
  diff: ProposalDiff!
}

type ProposalDiff {
  hunks: [DiffHunk!]!
  hasConflict: Boolean!
  stats: DiffStats!
}

type DiffStats {
  additions: Int!
  deletions: Int!
  hunksCount: Int!
}

type DiffHunk {
  index: Int!
  oldStart: Int!
  oldLines: Int!
  newStart: Int!
  newLines: Int!
  header: String!       # e.g. "@@ -10,5 +10,8 @@"
  lines: [DiffLine!]!
}

type DiffLine {
  type: DiffLineType!
  content: String!
  oldLineNo: Int
  newLineNo: Int
}

enum DiffLineType {
  CONTEXT
  ADD
  DELETE
}

# =============================================================================
# PROPOSAL INPUTS
# =============================================================================

input ProposeDocumentUpdateInput {
  vaultId: ID!
  path: String!                 # identify document by path
  proposedContent: String!
  description: String
  source: ProposalSource        # defaults to AI_SUGGESTED
}

input ApproveHunksInput {
  proposalId: ID!
  hunkIndexes: [Int!]!         # indexes of hunks to accept
  notes: String
}

# =============================================================================
# QUERIES
# =============================================================================

# NOTE: The design uses `extend type` for illustration. In the implementation,
# these are merged into the single schema.graphqls file.

extend type Query {
  # Get a single proposal with computed diff
  proposal(id: ID!): DocumentProposal

  # List proposals for a vault, optionally filtered by status
  proposals(vaultId: ID!, status: ProposalStatus): [DocumentProposal!]!
}

# Also: add a `proposals` field to the Document type for per-document listing
extend type Document {
  proposals(status: ProposalStatus): [DocumentProposal!]!
}

# =============================================================================
# MUTATIONS
# =============================================================================

extend type Mutation {
  # Agent proposes an update to a document
  proposeDocumentUpdate(input: ProposeDocumentUpdateInput!): DocumentProposal!

  # Approve entire proposal → applies full proposed content
  approveProposal(id: ID!, notes: String): Document!

  # Approve specific hunks → merges selected changes
  approveProposalHunks(input: ApproveHunksInput!): Document!

  # Reject entire proposal
  rejectProposal(id: ID!, notes: String): Boolean!
}
```

---

## Database Queries: `internal/db/queries_proposal.go`

```go
func (c *Client) CreateProposal(ctx context.Context, input models.DocumentProposalInput) (*models.DocumentProposal, error)
func (c *Client) GetProposal(ctx context.Context, id string) (*models.DocumentProposal, error)
func (c *Client) ListProposals(ctx context.Context, vaultID string, status *string) ([]models.DocumentProposal, error)
func (c *Client) ListProposalsByDocument(ctx context.Context, documentID string, status *string) ([]models.DocumentProposal, error)
func (c *Client) UpdateProposalStatus(ctx context.Context, id string, status models.ProposalStatus, notes *string) error
```

---

## Implementation Order

### Step 1: Data layer
1. `internal/models/proposal.go` — Go structs
2. `internal/db/schema.go` — Add `document_proposal` DDL
3. `internal/db/queries_proposal.go` — CRUD queries

### Step 2: Diff engine
4. `internal/review/diff.go` — `ComputeHunks`, `ApplyHunks`, `DiffStats`
5. `internal/review/diff_test.go` — Unit tests for diff/merge logic

### Step 3: Service layer
6. `internal/review/service.go` — Business logic (create, diff, approve, reject)

### Step 4: GraphQL API
7. `internal/graph/schema.graphqls` — Add types, queries, mutations
8. `internal/graph/helpers.go` — `proposalToGraphQL`, `hunkToGraphQL`, etc.
9. Run `just generate`
10. `internal/graph/schema.resolvers.go` — Implement resolvers
11. `internal/graph/resolver.go` — Wire up `review.Service`

### Step 5: Integration test (TODO)
12. `internal/integration/proposal_test.go` — Full lifecycle test (not yet implemented)

---

## Example API Usage

### Agent proposes an update

```graphql
mutation {
  proposeDocumentUpdate(input: {
    vaultId: "vault:abc123"
    path: "docs/architecture.md"
    proposedContent: "# Architecture\n\nUpdated content..."
    description: "Added section on caching layer and updated deployment diagram"
    source: AI_SUGGESTED
  }) {
    id
    status
    diff {
      stats { additions deletions hunksCount }
      hunks {
        index
        header
        lines { type content }
      }
    }
  }
}
```

### Web UI fetches pending proposals

```graphql
query {
  proposals(vaultId: "vault:abc123", status: PENDING) {
    id
    document { path title }
    description
    createdAt
    diff {
      stats { additions deletions hunksCount }
    }
  }
}
```

### User approves specific hunks

```graphql
mutation {
  approveProposalHunks(input: {
    proposalId: "document_proposal:xyz789"
    hunkIndexes: [0, 2, 3]
    notes: "Accepted caching section but not the deployment changes"
  }) {
    id
    path
    content
    updatedAt
  }
}
```

### User approves all changes

```graphql
mutation {
  approveProposal(id: "document_proposal:xyz789") {
    id
    path
    content
  }
}
```

---

## Key Design Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Store full proposed content | Yes | Diffs are computed on read — simpler, no stale diffs |
| Conflict detection | Hash comparison | `original_hash` vs current `content_hash` |
| Diff library | `pmezard/go-difflib` | Already indirect dep, standard unified diff |
| Hunk granularity | Line-level | Like git — most precise, familiar to developers |
| Partial approval merge | Accepted-hunk-only apply | Walk hunks in order, emit context+added for accepted, context+deleted for rejected |
| Review UI | Separate web project | API-first; GraphQL provides all diff data for any frontend |
| Cascade delete | SurrealDB event | Proposals auto-deleted when document is deleted |
| Vault scoping | Via document | Proposals inherit vault scope from their document |
