package db

// SchemaSQL contains the database schema initialization SQL.
// Matches Python memcp/db.py SCHEMA_SQL exactly.
const SchemaSQL = `
    -- ==========================================================================
    -- ENTITY TABLE
    -- ==========================================================================
    DEFINE TABLE IF NOT EXISTS entity SCHEMAFULL;
    DEFINE FIELD IF NOT EXISTS type ON entity TYPE string;
    -- TODO: Use set<string> when Go SDK supports CBOR tag 56 (v3.0 set type)
    DEFINE FIELD IF NOT EXISTS labels ON entity TYPE array<string>;
    DEFINE FIELD IF NOT EXISTS content ON entity TYPE string;
    DEFINE FIELD IF NOT EXISTS embedding ON entity TYPE array<float>;
    DEFINE FIELD IF NOT EXISTS confidence ON entity TYPE float DEFAULT 1.0;
    DEFINE FIELD IF NOT EXISTS source ON entity TYPE option<string>;
    DEFINE FIELD IF NOT EXISTS decay_weight ON entity TYPE float DEFAULT 1.0;
    DEFINE FIELD IF NOT EXISTS created ON entity TYPE datetime DEFAULT time::now();
    DEFINE FIELD IF NOT EXISTS accessed ON entity TYPE datetime DEFAULT time::now();
    DEFINE FIELD IF NOT EXISTS access_count ON entity TYPE int DEFAULT 0;
    -- Project namespacing: isolate memories by context
    DEFINE FIELD IF NOT EXISTS context ON entity TYPE option<string>;
    -- Importance scoring: heuristic-based salience
    DEFINE FIELD IF NOT EXISTS importance ON entity TYPE float DEFAULT 0.5;
    DEFINE FIELD IF NOT EXISTS user_importance ON entity TYPE option<float>;

    DEFINE INDEX IF NOT EXISTS entity_labels ON entity FIELDS labels;
    DEFINE INDEX IF NOT EXISTS entity_context ON entity FIELDS context;
    DEFINE INDEX IF NOT EXISTS entity_embedding ON entity FIELDS embedding HNSW DIMENSION 384 DIST COSINE TYPE F32;
    DEFINE ANALYZER IF NOT EXISTS entity_analyzer TOKENIZERS class FILTERS lowercase, ascii, snowball(english);
    DEFINE INDEX IF NOT EXISTS entity_content_ft ON entity FIELDS content FULLTEXT ANALYZER entity_analyzer BM25;

    -- ==========================================================================
    -- RELATIONS TABLE
    -- ==========================================================================
    -- Relation table with unique constraint to prevent duplicate edges
    -- Uses single table with rel_type field instead of dynamic table names
    DEFINE TABLE IF NOT EXISTS relates TYPE RELATION IN entity OUT entity SCHEMAFULL;
    DEFINE FIELD IF NOT EXISTS rel_type ON relates TYPE string;
    DEFINE FIELD IF NOT EXISTS weight ON relates TYPE float DEFAULT 1.0;
    DEFINE FIELD IF NOT EXISTS created ON relates TYPE datetime DEFAULT time::now();
    -- Unique constraint: sorted [in, out, rel_type] prevents duplicate relations
    DEFINE FIELD IF NOT EXISTS unique_key ON relates VALUE <string>string::concat(array::sort([<string>in, <string>out]), rel_type);
    DEFINE INDEX IF NOT EXISTS unique_relation ON relates FIELDS unique_key UNIQUE;

    -- ==========================================================================
    -- EPISODE TABLE (Episodic Memory)
    -- ==========================================================================
    DEFINE TABLE IF NOT EXISTS episode SCHEMAFULL;
    DEFINE FIELD IF NOT EXISTS content ON episode TYPE string;
    DEFINE FIELD IF NOT EXISTS summary ON episode TYPE option<string>;
    DEFINE FIELD IF NOT EXISTS embedding ON episode TYPE array<float>;
    DEFINE FIELD IF NOT EXISTS metadata ON episode TYPE option<object> FLEXIBLE;
    DEFINE FIELD IF NOT EXISTS timestamp ON episode TYPE datetime DEFAULT time::now();
    DEFINE FIELD IF NOT EXISTS context ON episode TYPE option<string>;
    DEFINE FIELD IF NOT EXISTS created ON episode TYPE datetime DEFAULT time::now();
    DEFINE FIELD IF NOT EXISTS accessed ON episode TYPE datetime DEFAULT time::now();
    DEFINE FIELD IF NOT EXISTS access_count ON episode TYPE int DEFAULT 0;

    DEFINE INDEX IF NOT EXISTS episode_timestamp ON episode FIELDS timestamp;
    DEFINE INDEX IF NOT EXISTS episode_context ON episode FIELDS context;
    DEFINE INDEX IF NOT EXISTS episode_embedding ON episode FIELDS embedding HNSW DIMENSION 384 DIST COSINE TYPE F32;
    DEFINE ANALYZER IF NOT EXISTS episode_analyzer TOKENIZERS class FILTERS lowercase, ascii, snowball(english);
    DEFINE INDEX IF NOT EXISTS episode_content_ft ON episode FIELDS content FULLTEXT ANALYZER episode_analyzer BM25;

    -- ==========================================================================
    -- EXTRACTED_FROM RELATION (links entities to source episodes)
    -- ==========================================================================
    DEFINE TABLE IF NOT EXISTS extracted_from TYPE RELATION IN entity OUT episode SCHEMAFULL;
    DEFINE FIELD IF NOT EXISTS position ON extracted_from TYPE option<int>;
    DEFINE FIELD IF NOT EXISTS confidence ON extracted_from TYPE float DEFAULT 1.0;
    DEFINE FIELD IF NOT EXISTS created ON extracted_from TYPE datetime DEFAULT time::now();

    -- ==========================================================================
    -- PROCEDURE TABLE (Procedural Memory)
    -- ==========================================================================
    -- Stores step-by-step workflows/processes with ordered steps
    DEFINE TABLE IF NOT EXISTS procedure SCHEMAFULL;
    DEFINE FIELD IF NOT EXISTS name ON procedure TYPE string;
    DEFINE FIELD IF NOT EXISTS description ON procedure TYPE string;
    DEFINE FIELD IF NOT EXISTS steps ON procedure TYPE array<object> FLEXIBLE;  -- [{order, content, optional}]
    -- Note: Must REMOVE then DEFINE to ensure FLEXIBLE is set (IF NOT EXISTS won't update existing field)
    REMOVE FIELD IF EXISTS steps.* ON procedure;
    DEFINE FIELD steps.* ON procedure TYPE object FLEXIBLE;  -- Allow nested object properties
    DEFINE FIELD IF NOT EXISTS embedding ON procedure TYPE array<float>;
    DEFINE FIELD IF NOT EXISTS context ON procedure TYPE option<string>;
    -- TODO: Use set<string> when Go SDK supports CBOR tag 56 (v3.0 set type)
    DEFINE FIELD IF NOT EXISTS labels ON procedure TYPE array<string>;
    DEFINE FIELD IF NOT EXISTS created ON procedure TYPE datetime DEFAULT time::now();
    DEFINE FIELD IF NOT EXISTS accessed ON procedure TYPE datetime DEFAULT time::now();
    DEFINE FIELD IF NOT EXISTS access_count ON procedure TYPE int DEFAULT 0;

    DEFINE INDEX IF NOT EXISTS procedure_context ON procedure FIELDS context;
    DEFINE INDEX IF NOT EXISTS procedure_labels ON procedure FIELDS labels;
    DEFINE INDEX IF NOT EXISTS procedure_embedding ON procedure FIELDS embedding HNSW DIMENSION 384 DIST COSINE TYPE F32;
    DEFINE ANALYZER IF NOT EXISTS procedure_analyzer TOKENIZERS class FILTERS lowercase, ascii, snowball(english);
    DEFINE INDEX IF NOT EXISTS procedure_name_ft ON procedure FIELDS name FULLTEXT ANALYZER procedure_analyzer BM25;
    DEFINE INDEX IF NOT EXISTS procedure_desc_ft ON procedure FIELDS description FULLTEXT ANALYZER procedure_analyzer BM25;

    -- ==========================================================================
    -- TYPE INDEX (for entity type ontology queries)
    -- ==========================================================================
    DEFINE INDEX IF NOT EXISTS entity_type ON entity FIELDS type;
`
