package db

import "fmt"

// SchemaSQL returns the database schema initialization SQL.
// The dimension parameter configures HNSW vector index dimensions.
func SchemaSQL(dimension int) string {
	return fmt.Sprintf(`
    -- ==========================================================================
    -- ANALYZER (shared across fulltext indexes)
    -- ==========================================================================
    DEFINE ANALYZER IF NOT EXISTS knowhow_analyzer
        TOKENIZERS class
        FILTERS lowercase, ascii, snowball(english);

    -- ==========================================================================
    -- USER TABLE
    -- ==========================================================================
    DEFINE TABLE IF NOT EXISTS user SCHEMAFULL;

    DEFINE FIELD IF NOT EXISTS name       ON user TYPE string;
    DEFINE FIELD IF NOT EXISTS email      ON user TYPE option<string>;
    DEFINE FIELD IF NOT EXISTS created_at ON user TYPE datetime DEFAULT time::now();

    DEFINE INDEX IF NOT EXISTS idx_user_name ON user FIELDS name UNIQUE;

    -- ==========================================================================
    -- VAULT TABLE
    -- ==========================================================================
    DEFINE TABLE IF NOT EXISTS vault SCHEMAFULL;

    DEFINE FIELD IF NOT EXISTS name        ON vault TYPE string;
    DEFINE FIELD IF NOT EXISTS description ON vault TYPE option<string>;
    DEFINE FIELD IF NOT EXISTS created_by  ON vault TYPE record<user>;
    DEFINE FIELD IF NOT EXISTS created_at  ON vault TYPE datetime DEFAULT time::now();
    DEFINE FIELD IF NOT EXISTS updated_at  ON vault TYPE datetime VALUE time::now();

    DEFINE INDEX IF NOT EXISTS idx_vault_name ON vault FIELDS name UNIQUE;

    -- ==========================================================================
    -- DOCUMENT TABLE
    -- ==========================================================================
    DEFINE TABLE IF NOT EXISTS document SCHEMAFULL;

    DEFINE FIELD IF NOT EXISTS vault        ON document TYPE record<vault>;
    DEFINE FIELD IF NOT EXISTS path         ON document TYPE string;
    DEFINE FIELD IF NOT EXISTS title        ON document TYPE string;
    DEFINE FIELD IF NOT EXISTS content      ON document TYPE string;
    DEFINE FIELD IF NOT EXISTS content_body ON document TYPE string;
    DEFINE FIELD IF NOT EXISTS labels       ON document TYPE array<string> DEFAULT [];
    DEFINE FIELD IF NOT EXISTS doc_type     ON document TYPE option<string>;
    DEFINE FIELD IF NOT EXISTS source       ON document TYPE string DEFAULT "manual";
    DEFINE FIELD IF NOT EXISTS source_path  ON document TYPE option<string>;
    DEFINE FIELD IF NOT EXISTS content_hash ON document TYPE option<string>;
    DEFINE FIELD IF NOT EXISTS metadata   ON document TYPE option<object> FLEXIBLE;
    DEFINE FIELD IF NOT EXISTS created_at ON document TYPE datetime DEFAULT time::now();
    DEFINE FIELD IF NOT EXISTS updated_at ON document TYPE datetime VALUE time::now();

    DEFINE INDEX IF NOT EXISTS idx_document_vault_path    ON document FIELDS vault, path UNIQUE;
    DEFINE INDEX IF NOT EXISTS idx_document_labels        ON document FIELDS labels;
    DEFINE INDEX IF NOT EXISTS idx_document_vault_doctype ON document FIELDS vault, doc_type;

    -- Cascade delete chunks, wiki_links, and versions when document deleted.
    -- ASYNC: runs after commit, eventually consistent. If all retries fail,
    -- orphaned records may remain — acceptable trade-off for write performance.
    DEFINE EVENT IF NOT EXISTS cascade_delete_document_chunks ON document
    WHEN $event = "DELETE" ASYNC RETRY 3 THEN {
        DELETE FROM chunk WHERE document = $before.id
    };

    DEFINE EVENT IF NOT EXISTS cascade_delete_document_versions ON document
    WHEN $event = "DELETE" ASYNC RETRY 3 THEN {
        DELETE FROM document_version WHERE document = $before.id
    };

    DEFINE EVENT IF NOT EXISTS cascade_delete_document_wiki_links ON document
    WHEN $event = "DELETE" ASYNC RETRY 3 THEN {
        DELETE FROM wiki_link WHERE from_doc = $before.id OR to_doc = $before.id
    };

    DEFINE EVENT IF NOT EXISTS cascade_delete_doc_relations ON document
    WHEN $event = "DELETE" ASYNC RETRY 3 THEN {
        DELETE FROM doc_relation WHERE in = $before.id OR out = $before.id
    };

    -- ==========================================================================
    -- DOCUMENT_VERSION TABLE
    -- ==========================================================================
    DEFINE TABLE IF NOT EXISTS document_version SCHEMAFULL;

    DEFINE FIELD IF NOT EXISTS document     ON document_version TYPE record<document>;
    DEFINE FIELD IF NOT EXISTS vault        ON document_version TYPE record<vault>;
    DEFINE FIELD IF NOT EXISTS version      ON document_version TYPE int;
    DEFINE FIELD IF NOT EXISTS content      ON document_version TYPE string;
    DEFINE FIELD IF NOT EXISTS content_hash ON document_version TYPE string;
    DEFINE FIELD IF NOT EXISTS title        ON document_version TYPE string;
    DEFINE FIELD IF NOT EXISTS source       ON document_version TYPE string DEFAULT "manual";
    DEFINE FIELD IF NOT EXISTS created_at   ON document_version TYPE datetime DEFAULT time::now();

    DEFINE INDEX IF NOT EXISTS idx_version_document        ON document_version FIELDS document;
    DEFINE INDEX IF NOT EXISTS idx_version_document_version ON document_version FIELDS document, version UNIQUE;

    -- ==========================================================================
    -- FOLDER TABLE
    -- ==========================================================================
    DEFINE TABLE IF NOT EXISTS folder SCHEMAFULL;

    DEFINE FIELD IF NOT EXISTS vault      ON folder TYPE record<vault>;
    DEFINE FIELD IF NOT EXISTS path       ON folder TYPE string;
    DEFINE FIELD IF NOT EXISTS name       ON folder TYPE string;
    DEFINE FIELD IF NOT EXISTS created_at ON folder TYPE datetime DEFAULT time::now();

    DEFINE INDEX IF NOT EXISTS idx_folder_vault_path ON folder FIELDS vault, path UNIQUE;
    DEFINE INDEX IF NOT EXISTS idx_folder_vault      ON folder FIELDS vault;

    -- ==========================================================================
    -- CHUNK TABLE
    -- ==========================================================================
    DEFINE TABLE IF NOT EXISTS chunk SCHEMAFULL;

    DEFINE FIELD IF NOT EXISTS document     ON chunk TYPE record<document>;
    DEFINE FIELD IF NOT EXISTS content      ON chunk TYPE string;
    DEFINE FIELD IF NOT EXISTS position     ON chunk TYPE int;
    DEFINE FIELD IF NOT EXISTS heading_path ON chunk TYPE option<string>;
    DEFINE FIELD IF NOT EXISTS labels       ON chunk TYPE array<string> DEFAULT [];
    DEFINE FIELD IF NOT EXISTS embedding    ON chunk TYPE option<array<float>>;
    DEFINE FIELD IF NOT EXISTS embed_at     ON chunk TYPE option<datetime>;
    DEFINE FIELD IF NOT EXISTS created_at   ON chunk TYPE datetime DEFAULT time::now();

    DEFINE INDEX IF NOT EXISTS idx_chunk_document   ON chunk FIELDS document;
    DEFINE INDEX IF NOT EXISTS idx_chunk_embed_at   ON chunk FIELDS embed_at;
    DEFINE INDEX IF NOT EXISTS idx_chunk_content_ft ON chunk FIELDS content FULLTEXT ANALYZER knowhow_analyzer BM25;
    DEFINE INDEX IF NOT EXISTS idx_chunk_embedding  ON chunk FIELDS embedding
        HNSW DIMENSION %d DIST COSINE TYPE F32 EFC 150 M 12 HASHED_VECTOR;

    -- ==========================================================================
    -- WIKI_LINK TABLE
    -- ==========================================================================
    DEFINE TABLE IF NOT EXISTS wiki_link SCHEMAFULL;

    DEFINE FIELD IF NOT EXISTS from_doc   ON wiki_link TYPE record<document>;
    DEFINE FIELD IF NOT EXISTS to_doc     ON wiki_link TYPE option<record<document>>;
    DEFINE FIELD IF NOT EXISTS raw_target ON wiki_link TYPE string;
    DEFINE FIELD IF NOT EXISTS vault      ON wiki_link TYPE record<vault>;

    DEFINE INDEX IF NOT EXISTS idx_wiki_link_from_doc         ON wiki_link FIELDS from_doc;
    DEFINE INDEX IF NOT EXISTS idx_wiki_link_vault            ON wiki_link FIELDS vault;
    DEFINE INDEX IF NOT EXISTS idx_wiki_link_vault_raw_target ON wiki_link FIELDS vault, raw_target;

    -- ==========================================================================
    -- DOC_RELATION TABLE (RELATION: document -> document)
    -- ==========================================================================
    DEFINE TABLE IF NOT EXISTS doc_relation SCHEMAFULL TYPE RELATION FROM document TO document;

    DEFINE FIELD IF NOT EXISTS rel_type   ON doc_relation TYPE string;
    DEFINE FIELD IF NOT EXISTS source     ON doc_relation TYPE string DEFAULT "manual";
    DEFINE FIELD IF NOT EXISTS created_at ON doc_relation TYPE datetime DEFAULT time::now();

    DEFINE INDEX IF NOT EXISTS idx_doc_relation_unique ON doc_relation FIELDS in, out, rel_type UNIQUE;

    -- ==========================================================================
    -- TEMPLATE TABLE
    -- ==========================================================================
    DEFINE TABLE IF NOT EXISTS template SCHEMAFULL;

    DEFINE FIELD IF NOT EXISTS vault          ON template TYPE option<record<vault>>;
    DEFINE FIELD IF NOT EXISTS name           ON template TYPE string;
    DEFINE FIELD IF NOT EXISTS description    ON template TYPE option<string>;
    DEFINE FIELD IF NOT EXISTS content        ON template TYPE string;
    DEFINE FIELD IF NOT EXISTS is_ai_template ON template TYPE bool DEFAULT false;
    DEFINE FIELD IF NOT EXISTS created_at     ON template TYPE datetime DEFAULT time::now();
    DEFINE FIELD IF NOT EXISTS updated_at     ON template TYPE datetime VALUE time::now();

    DEFINE INDEX IF NOT EXISTS idx_template_vault_name ON template FIELDS vault, name UNIQUE;

    -- ==========================================================================
    -- API_TOKEN TABLE
    -- ==========================================================================
    DEFINE TABLE IF NOT EXISTS api_token SCHEMAFULL;

    DEFINE FIELD IF NOT EXISTS user         ON api_token TYPE record<user>;
    DEFINE FIELD IF NOT EXISTS token_hash   ON api_token TYPE string;
    DEFINE FIELD IF NOT EXISTS name         ON api_token TYPE string;
    DEFINE FIELD IF NOT EXISTS vault_access ON api_token TYPE array<record<vault>> DEFAULT [];
    DEFINE FIELD IF NOT EXISTS last_used    ON api_token TYPE option<datetime>;
    DEFINE FIELD IF NOT EXISTS expires_at   ON api_token TYPE option<datetime>;
    DEFINE FIELD IF NOT EXISTS created_at   ON api_token TYPE datetime DEFAULT time::now();

    DEFINE INDEX IF NOT EXISTS idx_api_token_hash ON api_token FIELDS token_hash UNIQUE;

    -- ==========================================================================
    -- CONVERSATION TABLE
    -- ==========================================================================
    DEFINE TABLE IF NOT EXISTS conversation SCHEMAFULL;

    DEFINE FIELD IF NOT EXISTS vault      ON conversation TYPE record<vault>;
    DEFINE FIELD IF NOT EXISTS user       ON conversation TYPE record<user>;
    DEFINE FIELD IF NOT EXISTS title      ON conversation TYPE string DEFAULT "New conversation";
    DEFINE FIELD IF NOT EXISTS created_at ON conversation TYPE datetime DEFAULT time::now();
    DEFINE FIELD IF NOT EXISTS updated_at ON conversation TYPE datetime VALUE time::now();

    DEFINE INDEX IF NOT EXISTS idx_conversation_vault ON conversation FIELDS vault;
    DEFINE INDEX IF NOT EXISTS idx_conversation_user  ON conversation FIELDS user;

    -- ==========================================================================
    -- MESSAGE TABLE
    -- ==========================================================================
    DEFINE TABLE IF NOT EXISTS message SCHEMAFULL;

    DEFINE FIELD IF NOT EXISTS conversation ON message TYPE record<conversation>;
    DEFINE FIELD IF NOT EXISTS role         ON message TYPE string;
    DEFINE FIELD IF NOT EXISTS content      ON message TYPE string;
    DEFINE FIELD IF NOT EXISTS doc_refs     ON message TYPE array<string> DEFAULT [];
    DEFINE FIELD IF NOT EXISTS tool_name    ON message TYPE option<string>;
    DEFINE FIELD IF NOT EXISTS tool_input   ON message TYPE option<string>;
    DEFINE FIELD IF NOT EXISTS tool_meta    ON message TYPE option<string>;
    DEFINE FIELD IF NOT EXISTS tool_call_id ON message TYPE option<string>;
    DEFINE FIELD IF NOT EXISTS tool_calls   ON message TYPE option<string>;
    DEFINE FIELD IF NOT EXISTS created_at   ON message TYPE datetime DEFAULT time::now();

    DEFINE INDEX IF NOT EXISTS idx_message_conversation ON message FIELDS conversation;

    -- Cascade: delete messages when conversation deleted
    DEFINE EVENT IF NOT EXISTS cascade_delete_conversation_messages ON conversation
    WHEN $event = "DELETE" ASYNC RETRY 3 THEN {
        DELETE FROM message WHERE conversation = $before.id
    };

    -- ==========================================================================
    -- SEARCH_QUERY TABLE (embedding cache)
    -- ==========================================================================
    DEFINE TABLE IF NOT EXISTS search_query SCHEMAFULL;

    DEFINE FIELD IF NOT EXISTS query             ON search_query TYPE string;
    DEFINE FIELD IF NOT EXISTS embedding         ON search_query TYPE array<float>;
    DEFINE FIELD IF NOT EXISTS hit_count         ON search_query TYPE int DEFAULT 1;
    DEFINE FIELD IF NOT EXISTS first_searched_at ON search_query TYPE datetime DEFAULT time::now();
    DEFINE FIELD IF NOT EXISTS last_searched_at  ON search_query TYPE datetime DEFAULT time::now();

    DEFINE INDEX IF NOT EXISTS idx_search_query_query ON search_query FIELDS query UNIQUE;
`, dimension)
}
