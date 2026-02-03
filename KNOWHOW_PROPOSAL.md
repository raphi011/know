# Knowhow Design Specification

## 1. Overview

**Knowhow** is a next-generation evolution of `memcp`, designed as a Personal Knowledge RAG (Retrieval-Augmented Generation) Database. It bridges the gap between static local Markdown files and dynamic AI intelligence, utilizing a hybrid approach of Vector Search, Graph Databases, and Tagging.

### 1.1. Goals

- Provide a unified knowledge store combining document, graph, and vector capabilities
- Enable semantic search across personal and team knowledge bases
- Support human-in-the-loop review for AI-generated and AI-suggested changes
- Maintain Markdown formatting for human readability
- Remain LLM-provider agnostic

### 1.2. Non-Goals

- Real-time sync with local filesystem (database is source of truth)
- Browser-based UI (CLI and TUI only for v1)
- Multi-tenant SaaS deployment (single-user or small team focus)
- Replacing external tools (Google Calendar, Todoist)—integration only

### 1.3. Core Philosophy

- **Database-Centric:** The source of truth is the **SurrealDB** instance. While data can be imported from local files, once ingested, the database manages the lifecycle and relationships.
- **Markdown Formatting:** Even though stored in a database, document content is maintained and retrieved as Markdown to preserve readability and structure.
- **Hybrid Intelligence:** Combines Vector embeddings (semantic search) with Graph connections (structural relationships) and Labels (taxonomy).
- **Agnostic Intelligence:** The system is decoupled from specific LLM providers, offering a generic interface for interchangeable backends.

---

## 2. Requirements

### 2.1. Functional Requirements

| ID | Requirement | Priority |
|----|-------------|----------|
| FR-1 | Ingest Markdown files from local filesystem | Must |
| FR-2 | Store documents with full-text, vector embeddings, and graph relationships | Must |
| FR-3 | Semantic search via vector similarity | Must |
| FR-4 | Graph traversal for relationship queries | Must |
| FR-5 | LLM-powered query synthesis and answer generation | Must |
| FR-6 | Human-in-the-loop review system for AI changes | Must |
| FR-7 | Task extraction from TODO markers | Should |
| FR-8 | Contradiction detection across documents | Should |
| FR-9 | External API integrations (Calendar, Email, Todoist) | Could |

### 2.2. Non-Functional Requirements

| ID | Requirement | Target |
|----|-------------|--------|
| NFR-1 | Query latency | < 2s for semantic search |
| NFR-2 | Ingestion throughput | 100 docs/min |
| NFR-3 | Authentication | OIDC + API Key support |
| NFR-4 | Authorization | Role-based (RBAC) |
| NFR-5 | Data isolation | Privacy scopes (private/shared) |

---

## 3. Architecture

### 3.1. System Overview

```
┌─────────────────────────────────────────────────────────────┐
│                        User Interface                        │
│                    ┌─────────┬─────────┐                    │
│                    │   CLI   │   TUI   │                    │
│                    └────┬────┴────┬────┘                    │
│                         │         │                          │
│  ┌──────────────────────┴─────────┴──────────────────────┐  │
│  │                   GraphQL API Layer                    │  │
│  │                      (gqlgen)                          │  │
│  └──────────────────────────┬────────────────────────────┘  │
│                              │                               │
│  ┌───────────────┬──────────┴──────────┬────────────────┐  │
│  │               │                      │                │  │
│  │  LLM Interface│   Review System      │   Task Engine  │  │
│  │   (Generic)   │   (Human-in-loop)    │                │  │
│  │               │                      │                │  │
│  └───────────────┴──────────────────────┴────────────────┘  │
│                              │                               │
│  ┌───────────────────────────┴───────────────────────────┐  │
│  │                      SurrealDB                         │  │
│  │         ┌──────────┬──────────┬──────────┐            │  │
│  │         │ Document │  Graph   │  Vector  │            │  │
│  │         │  Store   │  Store   │  Store   │            │  │
│  │         └──────────┴──────────┴──────────┘            │  │
│  └───────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────┘
```

### 3.2. Database Layer (SurrealDB)

The backend relies on **SurrealDB** as the primary and only persistent storage:

- **Document Store:** Stores the full Markdown content of every document, along with metadata and frontmatter.
- **Graph Store:** Maps relationships between documents (e.g., `related_to`, `references`, `contradicts`).
- **Vector Store:** Stores embeddings for chunks of text, enabling semantic RAG.

### 3.3. API Layer (GraphQL)

GraphQL via **gqlgen** provides:

- Type-safe API contract between CLI/TUI and backend
- Flexible queries combining document, graph, and vector operations
- Subscription support for real-time review updates
- Batch operations for bulk ingestion

### 3.4. LLM Interface

A generic interface enables integration with **any** LLM provider. We use **[langchaingo](https://github.com/tmc/langchaingo)** — the most mature and widely-adopted Go LLM library.

#### 3.4.1. Why langchaingo?

| Criteria | langchaingo | Alternatives |
|----------|-------------|--------------|
| Provider support | 10+ (OpenAI, Anthropic, Google, Ollama, Bedrock, Cohere, Mistral...) | gollm: 6, bellman: 5 |
| Maturity | 5k+ stars, active development, Go blog featured | Smaller communities |
| Tool calling | ✅ Full support with `Tool`, `ToolCall` types | Varies |
| Streaming | ✅ Callback-based `WithStreamingFunc` | Varies |
| Embeddings | ✅ Separate `Embedder` interface | Often bundled |
| Reasoning models | ✅ `ThinkingMode` for Claude/DeepSeek | Rarely supported |

#### 3.4.2. Core Interfaces (from langchaingo)

**LLM Completions:**
```go
// github.com/tmc/langchaingo/llms
type Model interface {
    GenerateContent(ctx context.Context, messages []MessageContent, options ...CallOption) (*ContentResponse, error)
}

// Usage with streaming and tools
response, err := llm.GenerateContent(ctx, messages,
    llms.WithMaxTokens(4096),
    llms.WithTools(tools),
    llms.WithStreamingFunc(func(ctx context.Context, chunk []byte) error {
        // Handle streaming chunk
        return nil
    }),
)
```

**Embeddings:**
```go
// github.com/tmc/langchaingo/embeddings
type Embedder interface {
    EmbedDocuments(ctx context.Context, texts []string) ([][]float32, error)
    EmbedQuery(ctx context.Context, text string) ([]float32, error)
}
```

#### 3.4.3. Supported Providers

| Provider | Package | Use Case |
|----------|---------|----------|
| OpenAI | `langchaingo/llms/openai` | GPT-4, embeddings (ada-002) |
| Anthropic | `langchaingo/llms/anthropic` | Claude 3.5, tool use |
| Google | `langchaingo/llms/googleai` | Gemini models |
| Ollama | `langchaingo/llms/ollama` | Local models, privacy-sensitive |
| AWS Bedrock | `langchaingo/llms/bedrock` | Enterprise deployments |
| Mistral | `langchaingo/llms/mistral` | Open-weight models |

#### 3.4.4. Responsibilities

The LLM layer acts as the reasoning engine:

- **Query Planning:** Analyzing user prompts to generate optimized SurrealDB queries (combining Vector Search and Graph Traversal)
- **Synthesis:** Summarizing and reconciling data retrieved from the database to answer the user's question
- **Context Management:** Managing prompt construction, token limits, and template application

### 3.5. Security & Authorization

To enable safe exposure to the public internet, a robust security layer is implemented:

- **API Gateway:** A reverse proxy handling rate limiting, SSL termination, and request validation before traffic reaches the core application

---

## 4. Technology Stack

| Layer | Technology | Purpose |
|-------|------------|---------|
| Database | SurrealDB | Document, graph, and vector storage in single DB |
| Language | Go | Backend implementation, CLI, TUI |
| API | GraphQL (gqlgen) | Type-safe API with code generation |
| TUI Framework | Bubbletea | Elm-architecture terminal UI |
| TUI Styling | Lipgloss | Terminal styling and layout |
| TUI Components | Bubbles | Pre-built Bubbletea components |
| Markdown Render | Glamour | Terminal Markdown rendering |
| LLM Integration | [langchaingo](https://github.com/tmc/langchaingo) | Unified LLM/embedding interface, 10+ providers |

---

## 5. Data Model

### 5.1. Document Schema

```surql
DEFINE TABLE document SCHEMAFULL;

DEFINE FIELD id ON document TYPE string;
DEFINE FIELD title ON document TYPE string;
DEFINE FIELD content ON document TYPE string;           -- Full Markdown
DEFINE FIELD frontmatter ON document TYPE object;       -- Parsed YAML
DEFINE FIELD labels ON document TYPE array<string>;
DEFINE FIELD source_path ON document TYPE option<string>;
DEFINE FIELD doc_type ON document TYPE string;          -- user | ai_generated
DEFINE FIELD created_at ON document TYPE datetime;
DEFINE FIELD updated_at ON document TYPE datetime;
DEFINE FIELD privacy_scope ON document TYPE string;     -- private | shared

DEFINE INDEX idx_labels ON document FIELDS labels;
DEFINE INDEX idx_doc_type ON document FIELDS doc_type;
```

### 5.2. Graph Relationships

```surql
DEFINE TABLE related_to SCHEMAFULL TYPE RELATION FROM document TO document;
DEFINE FIELD strength ON related_to TYPE float;
DEFINE FIELD reason ON related_to TYPE string;

DEFINE TABLE references SCHEMAFULL TYPE RELATION FROM document TO document;
DEFINE FIELD context ON references TYPE string;

DEFINE TABLE contradicts SCHEMAFULL TYPE RELATION FROM document TO document;
DEFINE FIELD description ON contradicts TYPE string;
DEFINE FIELD detected_at ON contradicts TYPE datetime;
DEFINE FIELD resolved ON contradicts TYPE bool DEFAULT false;
```

### 5.3. Chunk & Embedding Schema

```surql
DEFINE TABLE chunk SCHEMAFULL;

DEFINE FIELD id ON chunk TYPE string;
DEFINE FIELD document ON chunk TYPE record<document>;
DEFINE FIELD content ON chunk TYPE string;
DEFINE FIELD position ON chunk TYPE int;
DEFINE FIELD embedding ON chunk TYPE array<float>;

DEFINE INDEX idx_chunk_embedding ON chunk FIELDS embedding MTREE DIMENSION 1536;
```

### 5.4. Review Queue Schema

```surql
DEFINE TABLE review_item SCHEMAFULL;

DEFINE FIELD id ON review_item TYPE string;
DEFINE FIELD item_type ON review_item TYPE string;      -- document | relationship | task
DEFINE FIELD action ON review_item TYPE string;         -- create | update | delete
DEFINE FIELD target_id ON review_item TYPE option<string>;
DEFINE FIELD proposed_data ON review_item TYPE object;
DEFINE FIELD original_data ON review_item TYPE option<object>;
DEFINE FIELD source ON review_item TYPE string;         -- ai_generated | ai_suggested | import
DEFINE FIELD status ON review_item TYPE string;         -- pending | accepted | rejected
DEFINE FIELD created_at ON review_item TYPE datetime;
DEFINE FIELD reviewed_at ON review_item TYPE option<datetime>;
DEFINE FIELD reviewer_notes ON review_item TYPE option<string>;

DEFINE INDEX idx_review_status ON review_item FIELDS status;
DEFINE INDEX idx_review_created ON review_item FIELDS created_at;
```

---

## 6. Features

### 6.1. Data Ingestion & Indexing

- **Import/Scrape:** The CLI facilitates migration of existing local Markdown folders into the database.
  - Command: `knowhow scrape .`
  - Recursively finds `.md` files, parses them, and uploads content to SurrealDB
  - Once scraped, changes to local files are not automatically reflected unless re-scraped
- **Chunking:** Intelligently splits long documents into semantically meaningful chunks for embedding
- **Enrichment:** Automatically generates graph edges and labels during ingestion

### 6.2. Document Types

- **User-Generated:** Manual notes, documentation, journals
- **AI-Generated:** Synthesized answers, summaries, and research reports
- **Templating:** Configurable templates ensure AI-generated files adhere to specific structure (e.g., "Daily Summary", "Technical Spec")

### 6.3. Intelligent Analysis

- **Contradiction Detection:** Actively scans for conflicting information across documents (e.g., different dates for same event, conflicting definitions) and flags them with `contradicts` edges
- **Consensus Building:** When asked a question with conflicting sources, highlights the discrepancy rather than hallucinating a single truth

### 6.4. Task Management

- **Extraction:** Automatically parses `- [ ]` check items and "TODO" markers from notes, converting them into structured task objects
- **Contextual Prioritization:** Reorders tasks based on projects or deadlines mentioned in surrounding text or linked documents
- **Entity Linking:** Associates tasks with relevant people and projects found in the graph, enabling queries like "What do I owe [Person Name]?"

### 6.5. Review System

Human-in-the-loop approval workflow for AI changes.

#### 6.5.1. Scope

Items requiring review:

- **AI-Generated Content:** New documents created by LLM synthesis
- **AI-Suggested Changes:** Updates to existing documents proposed by analysis
- **Relationship Inference:** Graph edges detected automatically
- **Bulk Imports:** Large ingestion operations (optional)

#### 6.5.2. Workflow

```
┌─────────────┐    ┌──────────────┐    ┌─────────────┐
│  AI Action  │───▶│ Review Queue │───▶│   Review    │
└─────────────┘    └──────────────┘    │   (TUI)     │
                                       └──────┬──────┘
                                              │
                   ┌──────────────────────────┼──────────────────────────┐
                   │                          │                          │
                   ▼                          ▼                          ▼
            ┌──────────┐              ┌──────────────┐            ┌──────────┐
            │  Accept  │              │    Reject    │            │  Refine  │
            │  (Apply) │              │   (Discard)  │            │(Re-prompt)│
            └──────────┘              └──────────────┘            └────┬─────┘
                                                                       │
                                                                       ▼
                                                                 ┌──────────┐
                                                                 │   LLM    │
                                                                 │ Re-query │
                                                                 └──────────┘
```

#### 6.5.3. Review Actions

| Action | Description |
|--------|-------------|
| **Accept** | Apply proposed change to database |
| **Reject** | Discard proposal, mark as rejected |
| **Refine** | Send back to LLM with additional instructions, creates new review item |

#### 6.5.4. Persistence

- All review items persisted in `review_item` table
- Full audit trail of decisions
- Rejected items retained for analysis (improve future AI suggestions)

### 6.6. External Integrations (Future)

- **Data Sync:** Two-way synchronization with external platforms to enrich knowledge base and automate actions
- **Supported Tools:**
  - **Google Calendar:** Fetch events to augment daily logs; create events from notes
  - **Gmail:** Index emails as knowledge; draft replies based on knowledge base
  - **Todoist:** Sync tasks extracted from notes; update task status when completed

---

## 7. User Interfaces

### 7.1. CLI Commands

Designed for quick ingestion and one-off queries. Running `knowhow` without a subcommand launches the interactive TUI.

| Command | Description |
|---------|-------------|
| `knowhow scrape <path>` | Index a directory into the knowledge base |
| `knowhow ask "<question>"` | One-off semantic query with synthesized answer |

### 7.2. TUI Features

The TUI is the primary interface for interacting with the knowledge base. It is launched by running `knowhow` with no arguments. In the initial version, the TUI will be focused on basic navigation and review.

- **Knowledge Explorer:** Basic navigation of documents and their relationships
- **Review Mode:** Initial interface for approving or rejecting AI-proposed changes
- **Basic Chat:** Simple multi-turn conversation with the knowledge base

---

## 8. Security

### 8.1. Authentication

- **OIDC Integration:** Standard providers (Google, GitHub)
- **API Keys:** For CLI/programmatic access
- **Session Management:** Token-based with configurable expiry

### 8.2. Authorization (RBAC)

| Role | Permissions |
|------|-------------|
| Owner | Full access to all data and settings |
| Editor | Read/write documents, cannot modify roles |
| Viewer | Read-only access to shared content |
| API | Scoped access via API key permissions |

### 8.3. Data Privacy Scopes

- **Private Knowledge:** Content accessible only by owning user (e.g., personal journals, drafts)
- **Shared Knowledge:** Content accessible to specific groups or all users, facilitating team collaboration

---

## 9. Roadmap

| Phase | Focus | Key Deliverables |
|-------|-------|------------------|
| 1 | Prototype | CLI (`scrape`, `ask`), basic TUI framework, SurrealDB schema |
| 2 | Core | Basic RAG implementation, ingestion enrichment, basic TUI navigation |
| 3 | Refinement | Advanced TUI features (Graph navigation, Review mode), multi-provider LLM support |
| 4 | Multi-User | RBAC, privacy scopes, API gateway |
| 5 | Ecosystem | External API integrations, advanced automation |

---

## 10. Open Questions / Future Considerations

- **Embedding Model:** Which embedding model to use? (OpenAI ada-002, Cohere, local?)
- **Chunk Strategy:** Optimal chunk size and overlap for Markdown documents?
- **Conflict Resolution:** How to handle merge conflicts when same document modified via multiple paths?
- **Offline Mode:** Should CLI support offline operation with later sync?
- **Export Format:** Standard format for exporting knowledge base? (Markdown folder, JSON, SQLite?)
- **Plugin System:** Allow user-defined LLM prompts and templates?
- **Version History:** Track document revisions or rely on external VCS?
