package db

import "fmt"

// SchemaSQL returns the database schema initialization SQL for Knowhow.
// Personal knowledge RAG database with flexible entity model.
// The dimension parameter configures HNSW vector index dimensions.
func SchemaSQL(dimension int) string {
	return fmt.Sprintf(`
    -- ==========================================================================
    -- ENTITY TABLE (Core - Flexible Knowledge Atom)
    -- ==========================================================================
    -- Everything is an entity: documents, people, services, concepts, tasks.
    -- Flexible schema allows storing any type of knowledge.
    DEFINE TABLE IF NOT EXISTS entity SCHEMAFULL;

    -- Identity
    DEFINE FIELD IF NOT EXISTS type ON entity TYPE string;              -- "person", "service", "document", "concept", "task", etc.
    DEFINE FIELD IF NOT EXISTS name ON entity TYPE string;              -- Display name/title

    -- Content (optional - not all entities need long content)
    DEFINE FIELD IF NOT EXISTS content ON entity TYPE option<string>;   -- Full text (Markdown)
    DEFINE FIELD IF NOT EXISTS summary ON entity TYPE option<string>;   -- Short description

    -- Organization
    DEFINE FIELD IF NOT EXISTS labels ON entity TYPE array<string> DEFAULT [];  -- Flexible tags ["work", "banking", "team-platform"]

    -- Quality & Trust
    DEFINE FIELD IF NOT EXISTS verified ON entity TYPE bool DEFAULT false;      -- Human-reviewed?
    DEFINE FIELD IF NOT EXISTS confidence ON entity TYPE float DEFAULT 0.5;     -- 0-1 certainty (for AI content)
    DEFINE FIELD IF NOT EXISTS source ON entity TYPE string DEFAULT "manual";   -- "manual" | "mcp" | "scrape" | "ai_generated"
    DEFINE FIELD IF NOT EXISTS source_path ON entity TYPE option<string>;       -- Original file path if scraped
    DEFINE FIELD IF NOT EXISTS content_hash ON entity TYPE option<string>;     -- SHA256 hash for skip-unchanged

    -- Type-specific data
    DEFINE FIELD IF NOT EXISTS metadata ON entity TYPE option<object> FLEXIBLE;

    -- Search
    DEFINE FIELD IF NOT EXISTS embedding ON entity TYPE option<array<float>>;   -- Computed from content/summary

    -- Timestamps
    DEFINE FIELD IF NOT EXISTS created_at ON entity TYPE datetime DEFAULT time::now();
    DEFINE FIELD IF NOT EXISTS updated_at ON entity TYPE datetime VALUE time::now();
    DEFINE FIELD IF NOT EXISTS accessed ON entity TYPE datetime DEFAULT time::now();
    DEFINE FIELD IF NOT EXISTS access_count ON entity TYPE int DEFAULT 0;

    -- Indexes
    DEFINE INDEX IF NOT EXISTS idx_entity_type ON entity FIELDS type;
    DEFINE INDEX IF NOT EXISTS idx_entity_labels ON entity FIELDS labels;
    DEFINE INDEX IF NOT EXISTS idx_entity_verified ON entity FIELDS verified;
    DEFINE INDEX IF NOT EXISTS idx_entity_source ON entity FIELDS source;
    DEFINE ANALYZER IF NOT EXISTS entity_analyzer TOKENIZERS class FILTERS lowercase, ascii, snowball(english);
    DEFINE INDEX IF NOT EXISTS idx_entity_content_ft ON entity FIELDS content FULLTEXT ANALYZER entity_analyzer BM25;
    DEFINE INDEX IF NOT EXISTS idx_entity_name_ft ON entity FIELDS name FULLTEXT ANALYZER entity_analyzer BM25;
    DEFINE INDEX IF NOT EXISTS idx_entity_embedding ON entity FIELDS embedding
        HNSW DIMENSION %d DIST COSINE TYPE F32 EFC 150 M 12;

    -- ==========================================================================
    -- CHUNK TABLE (RAG Pieces for Long Content)
    -- ==========================================================================
    -- Auto-generated for entities with long content (> threshold).
    DEFINE TABLE IF NOT EXISTS chunk SCHEMAFULL;

    DEFINE FIELD IF NOT EXISTS entity ON chunk TYPE record<entity>;     -- Parent reference
    DEFINE FIELD IF NOT EXISTS content ON chunk TYPE string;            -- Chunk text
    DEFINE FIELD IF NOT EXISTS position ON chunk TYPE int;              -- Order within entity
    DEFINE FIELD IF NOT EXISTS heading_path ON chunk TYPE option<string>; -- "## Setup > ### Install"
    DEFINE FIELD IF NOT EXISTS labels ON chunk TYPE array<string> DEFAULT []; -- Inherited from parent
    DEFINE FIELD IF NOT EXISTS embedding ON chunk TYPE array<float>;
    DEFINE FIELD IF NOT EXISTS created_at ON chunk TYPE datetime DEFAULT time::now();

    -- Indexes
    DEFINE INDEX IF NOT EXISTS idx_chunk_entity ON chunk FIELDS entity;
    DEFINE INDEX IF NOT EXISTS idx_chunk_labels ON chunk FIELDS labels;
    DEFINE ANALYZER IF NOT EXISTS chunk_analyzer TOKENIZERS class FILTERS lowercase, ascii, snowball(english);
    DEFINE INDEX IF NOT EXISTS idx_chunk_content_ft ON chunk FIELDS content FULLTEXT ANALYZER chunk_analyzer BM25;
    DEFINE INDEX IF NOT EXISTS idx_chunk_embedding ON chunk FIELDS embedding
        HNSW DIMENSION %d DIST COSINE TYPE F32 EFC 150 M 12;

    -- Cascade delete when parent entity deleted
    DEFINE EVENT IF NOT EXISTS cascade_delete_chunks ON entity
    WHEN $event = "DELETE" THEN {
        DELETE FROM chunk WHERE entity = $before.id
    };

    -- ==========================================================================
    -- TEMPLATE TABLE (Output Rendering Templates)
    -- ==========================================================================
    -- Templates for synthesizing/rendering output from accumulated knowledge.
    DEFINE TABLE IF NOT EXISTS template SCHEMAFULL;

    DEFINE FIELD IF NOT EXISTS name ON template TYPE string;            -- "Peer Review", "Service Summary", "Weekly Report"
    DEFINE FIELD IF NOT EXISTS description ON template TYPE option<string>;
    DEFINE FIELD IF NOT EXISTS content ON template TYPE string;         -- Markdown template with sections to fill
    DEFINE FIELD IF NOT EXISTS created_at ON template TYPE datetime DEFAULT time::now();
    DEFINE FIELD IF NOT EXISTS updated_at ON template TYPE datetime VALUE time::now();

    DEFINE INDEX IF NOT EXISTS idx_template_name ON template FIELDS name UNIQUE;

    -- ==========================================================================
    -- RELATES_TO RELATION (General Entity Relationships)
    -- ==========================================================================
    DEFINE TABLE IF NOT EXISTS relates_to SCHEMAFULL TYPE RELATION FROM entity TO entity;
    DEFINE FIELD IF NOT EXISTS rel_type ON relates_to TYPE string;      -- "works_on", "owns", "references", etc.
    DEFINE FIELD IF NOT EXISTS strength ON relates_to TYPE float DEFAULT 1.0;
    DEFINE FIELD IF NOT EXISTS source ON relates_to TYPE string DEFAULT "manual"; -- "manual" | "inferred" | "ai_detected"
    DEFINE FIELD IF NOT EXISTS metadata ON relates_to TYPE option<object> FLEXIBLE;
    DEFINE FIELD IF NOT EXISTS created_at ON relates_to TYPE datetime DEFAULT time::now();

    -- Unique constraint: prevent duplicate relations of same type between same entities
    DEFINE FIELD IF NOT EXISTS unique_key ON relates_to VALUE <string>string::concat(array::sort([<string>in, <string>out]), rel_type);
    DEFINE INDEX IF NOT EXISTS unique_relates_to ON relates_to FIELDS unique_key UNIQUE;

    -- Cascade delete relations when entity deleted
    DEFINE EVENT IF NOT EXISTS cascade_delete_relations ON entity
    WHEN $event = "DELETE" THEN {
        DELETE FROM relates_to WHERE in = $before.id OR out = $before.id
    };

    -- ==========================================================================
    -- CONTRADICTS RELATION (Contradiction Detection)
    -- ==========================================================================
    -- For AI-detected conflicts between entities.
    DEFINE TABLE IF NOT EXISTS contradicts SCHEMAFULL TYPE RELATION FROM entity TO entity;
    DEFINE FIELD IF NOT EXISTS explanation ON contradicts TYPE string;
    DEFINE FIELD IF NOT EXISTS confidence ON contradicts TYPE float;
    DEFINE FIELD IF NOT EXISTS resolved ON contradicts TYPE bool DEFAULT false;
    DEFINE FIELD IF NOT EXISTS detected_at ON contradicts TYPE datetime DEFAULT time::now();

    -- Cascade delete contradictions when entity deleted
    DEFINE EVENT IF NOT EXISTS cascade_delete_contradicts ON entity
    WHEN $event = "DELETE" THEN {
        DELETE FROM contradicts WHERE in = $before.id OR out = $before.id
    };

    -- ==========================================================================
    -- TOKEN_USAGE TABLE (Cost Tracking)
    -- ==========================================================================
    -- Track all LLM token consumption for cost monitoring and optimization.
    DEFINE TABLE IF NOT EXISTS token_usage SCHEMAFULL;

    DEFINE FIELD IF NOT EXISTS operation ON token_usage TYPE string;      -- "embed", "ask", "extract_graph", "render"
    DEFINE FIELD IF NOT EXISTS model ON token_usage TYPE string;          -- "gpt-4", "claude-3", "ollama/llama3"
    DEFINE FIELD IF NOT EXISTS input_tokens ON token_usage TYPE int;
    DEFINE FIELD IF NOT EXISTS output_tokens ON token_usage TYPE int;
    DEFINE FIELD IF NOT EXISTS total_tokens ON token_usage TYPE int;
    DEFINE FIELD IF NOT EXISTS cost_usd ON token_usage TYPE option<float>; -- Estimated cost (if known)
    DEFINE FIELD IF NOT EXISTS entity_id ON token_usage TYPE option<string>; -- Related entity if applicable
    DEFINE FIELD IF NOT EXISTS created_at ON token_usage TYPE datetime DEFAULT time::now();

    DEFINE INDEX IF NOT EXISTS idx_usage_operation ON token_usage FIELDS operation;
    DEFINE INDEX IF NOT EXISTS idx_usage_created ON token_usage FIELDS created_at;

    -- ==========================================================================
    -- INGEST_JOB TABLE (Async Job Persistence)
    -- ==========================================================================
    -- Persists async ingestion jobs for restart resilience.
    -- Jobs can be named for easy re-running and have curated labels.
    DEFINE TABLE IF NOT EXISTS ingest_job SCHEMAFULL;

    DEFINE FIELD IF NOT EXISTS job_type ON ingest_job TYPE string;
    DEFINE FIELD IF NOT EXISTS status ON ingest_job TYPE string;
    DEFINE FIELD IF NOT EXISTS name ON ingest_job TYPE option<string>;          -- User-provided name for rerunning
    DEFINE FIELD IF NOT EXISTS labels ON ingest_job TYPE array<string> DEFAULT [];  -- Curated labels applied to entities
    DEFINE FIELD IF NOT EXISTS dir_path ON ingest_job TYPE string;
    DEFINE FIELD IF NOT EXISTS files ON ingest_job TYPE array<string>;
    DEFINE FIELD IF NOT EXISTS options ON ingest_job TYPE option<object> FLEXIBLE;
    DEFINE FIELD IF NOT EXISTS total ON ingest_job TYPE int DEFAULT 0;
    DEFINE FIELD IF NOT EXISTS progress ON ingest_job TYPE int DEFAULT 0;
    DEFINE FIELD IF NOT EXISTS result ON ingest_job TYPE option<object> FLEXIBLE;
    DEFINE FIELD IF NOT EXISTS error ON ingest_job TYPE option<string>;
    DEFINE FIELD IF NOT EXISTS started_at ON ingest_job TYPE datetime DEFAULT time::now();
    DEFINE FIELD IF NOT EXISTS completed_at ON ingest_job TYPE option<datetime>;

    DEFINE INDEX IF NOT EXISTS idx_job_status ON ingest_job FIELDS status;
    DEFINE INDEX IF NOT EXISTS idx_job_name ON ingest_job FIELDS name UNIQUE;
`, dimension, dimension)
}
