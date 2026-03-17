package db

import "fmt"

// SchemaSQL returns the database schema initialization SQL.
// The dimension parameter configures HNSW vector index dimensions.
func SchemaSQL(dimension int) string {
	return fmt.Sprintf(`
    -- ==========================================================================
    -- ANALYZER (shared across fulltext indexes)
    -- ==========================================================================
    DEFINE ANALYZER IF NOT EXISTS know_analyzer
        TOKENIZERS class
        FILTERS lowercase, ascii, snowball(english);

    -- ==========================================================================
    -- USER TABLE
    -- ==========================================================================
    DEFINE TABLE IF NOT EXISTS user SCHEMAFULL;

    DEFINE FIELD IF NOT EXISTS name             ON user TYPE string;
    DEFINE FIELD IF NOT EXISTS email            ON user TYPE option<string>;
    DEFINE FIELD IF NOT EXISTS is_system_admin  ON user TYPE bool DEFAULT false;
    DEFINE FIELD IF NOT EXISTS created_at       ON user TYPE datetime DEFAULT time::now();

    DEFINE INDEX IF NOT EXISTS idx_user_name ON user FIELDS name UNIQUE;

    -- ==========================================================================
    -- VAULT TABLE
    -- ==========================================================================
    DEFINE TABLE IF NOT EXISTS vault SCHEMAFULL;

    DEFINE FIELD IF NOT EXISTS name        ON vault TYPE string;
    DEFINE FIELD IF NOT EXISTS description ON vault TYPE option<string>;
    DEFINE FIELD IF NOT EXISTS settings    ON vault TYPE option<object> FLEXIBLE;
    DEFINE FIELD IF NOT EXISTS created_by  ON vault TYPE record<user>;
    DEFINE FIELD IF NOT EXISTS created_at  ON vault TYPE datetime DEFAULT time::now();
    DEFINE FIELD IF NOT EXISTS updated_at  ON vault TYPE datetime VALUE time::now();

    DEFINE INDEX IF NOT EXISTS idx_vault_name ON vault FIELDS name UNIQUE;

    -- ==========================================================================
    -- FILE TABLE (unified: documents, assets, folders)
    -- ==========================================================================
    DEFINE TABLE IF NOT EXISTS file SCHEMAFULL;

    DEFINE FIELD IF NOT EXISTS vault          ON file TYPE record<vault>;
    DEFINE FIELD IF NOT EXISTS path           ON file TYPE string;
    DEFINE FIELD IF NOT EXISTS title          ON file TYPE string DEFAULT "";
    DEFINE FIELD IF NOT EXISTS is_folder      ON file TYPE bool DEFAULT false;
    DEFINE FIELD IF NOT EXISTS mime_type      ON file TYPE string DEFAULT "application/octet-stream";
    DEFINE FIELD IF NOT EXISTS content        ON file TYPE string DEFAULT "";
    DEFINE FIELD IF NOT EXISTS content_length ON file TYPE int DEFAULT 0;
    DEFINE FIELD IF NOT EXISTS labels         ON file TYPE array<string> DEFAULT [];
    DEFINE FIELD IF NOT EXISTS doc_type       ON file TYPE option<string>;
    DEFINE FIELD IF NOT EXISTS content_hash   ON file TYPE option<string>;
    DEFINE FIELD IF NOT EXISTS metadata       ON file TYPE option<object> FLEXIBLE;
    DEFINE FIELD IF NOT EXISTS size           ON file TYPE int DEFAULT 0;
    DEFINE FIELD IF NOT EXISTS last_accessed_at ON file TYPE option<datetime>;
    DEFINE FIELD IF NOT EXISTS access_count   ON file TYPE int DEFAULT 0;
    DEFINE FIELD IF NOT EXISTS created_at     ON file TYPE datetime DEFAULT time::now();
    DEFINE FIELD IF NOT EXISTS updated_at     ON file TYPE datetime VALUE time::now();
    DEFINE FIELD IF NOT EXISTS stem           ON file TYPE string DEFAULT "";

    DEFINE INDEX IF NOT EXISTS idx_file_vault_path    ON file FIELDS vault, path UNIQUE;
    DEFINE INDEX IF NOT EXISTS idx_file_labels        ON file FIELDS labels;
    DEFINE INDEX IF NOT EXISTS idx_file_vault_doctype ON file FIELDS vault, doc_type;
    DEFINE INDEX IF NOT EXISTS idx_file_vault_folder  ON file FIELDS vault, is_folder;
    DEFINE INDEX IF NOT EXISTS idx_file_vault_stem    ON file FIELDS vault, stem;

    -- Cascade delete chunks, versions, wiki_links, relations, tasks, labels
    -- when a non-folder file is deleted.
    DEFINE EVENT IF NOT EXISTS cascade_delete_file_chunks ON file
    WHEN $event = "DELETE" AND $before.is_folder = false ASYNC RETRY 3 THEN {
        DELETE FROM chunk WHERE file = $before.id
    };

    DEFINE EVENT IF NOT EXISTS cascade_delete_file_versions ON file
    WHEN $event = "DELETE" AND $before.is_folder = false ASYNC RETRY 3 THEN {
        DELETE FROM file_version WHERE file = $before.id
    };

    DEFINE EVENT IF NOT EXISTS cascade_delete_file_wiki_links ON file
    WHEN $event = "DELETE" AND $before.is_folder = false ASYNC RETRY 3 THEN {
        -- Outgoing links: delete (the file's own links)
        DELETE FROM wiki_link WHERE from_file = $before.id;
        -- Incoming links: unresolve (preserve the dangling reference)
        UPDATE wiki_link SET to_file = NONE WHERE to_file = $before.id
    };

    DEFINE EVENT IF NOT EXISTS cascade_delete_file_relations ON file
    WHEN $event = "DELETE" AND $before.is_folder = false ASYNC RETRY 3 THEN {
        DELETE FROM file_relation WHERE in = $before.id OR out = $before.id
    };

    DEFINE EVENT IF NOT EXISTS cascade_delete_file_tasks ON file
    WHEN $event = "DELETE" AND $before.is_folder = false ASYNC RETRY 3 THEN {
        DELETE FROM task WHERE file = $before.id
    };

    DEFINE EVENT IF NOT EXISTS cascade_delete_file_label_edges ON file
    WHEN $event = "DELETE" AND $before.is_folder = false ASYNC RETRY 3 THEN {
        DELETE FROM has_label WHERE in = $before.id
    };

    DEFINE EVENT IF NOT EXISTS cascade_delete_file_external_links ON file
    WHEN $event = "DELETE" AND $before.is_folder = false ASYNC RETRY 3 THEN {
        DELETE FROM external_link WHERE from_file = $before.id
    };

    -- Auto-create tombstone when a non-folder file is deleted
    DEFINE EVENT IF NOT EXISTS create_tombstone_on_delete ON file
    WHEN $event = "DELETE" AND $before.is_folder = false ASYNC RETRY 3 THEN {
        CREATE file_tombstone SET
            vault = $before.vault,
            file_id = type::string($before.id),
            path = $before.path
    };

    -- ==========================================================================
    -- FILE_VERSION TABLE (historical snapshots)
    -- ==========================================================================
    DEFINE TABLE IF NOT EXISTS file_version SCHEMAFULL;

    DEFINE FIELD IF NOT EXISTS file         ON file_version TYPE record<file>;
    DEFINE FIELD IF NOT EXISTS vault        ON file_version TYPE record<vault>;
    DEFINE FIELD IF NOT EXISTS version      ON file_version TYPE int;
    DEFINE FIELD IF NOT EXISTS content      ON file_version TYPE string;
    DEFINE FIELD IF NOT EXISTS content_hash ON file_version TYPE string;
    DEFINE FIELD IF NOT EXISTS title        ON file_version TYPE string;
    DEFINE FIELD IF NOT EXISTS created_at   ON file_version TYPE datetime DEFAULT time::now();

    DEFINE INDEX IF NOT EXISTS idx_version_file          ON file_version FIELDS file;
    DEFINE INDEX IF NOT EXISTS idx_version_file_version   ON file_version FIELDS file, version UNIQUE;
    DEFINE INDEX IF NOT EXISTS idx_version_vault          ON file_version FIELDS vault;

    -- ==========================================================================
    -- CHUNK TABLE (enhanced with multimodal support)
    -- ==========================================================================
    DEFINE TABLE IF NOT EXISTS chunk SCHEMAFULL;

    DEFINE FIELD IF NOT EXISTS file       ON chunk TYPE record<file>;
    DEFINE FIELD IF NOT EXISTS text       ON chunk TYPE string;
    DEFINE FIELD IF NOT EXISTS mime_type  ON chunk TYPE string DEFAULT "text/plain";
    DEFINE FIELD IF NOT EXISTS position   ON chunk TYPE int;
    DEFINE FIELD IF NOT EXISTS source_loc ON chunk TYPE option<string>;
    DEFINE FIELD IF NOT EXISTS labels     ON chunk TYPE array<string> DEFAULT [];
    DEFINE FIELD IF NOT EXISTS data_hash  ON chunk TYPE option<string>;
    DEFINE FIELD IF NOT EXISTS embedding  ON chunk TYPE option<array<float>>;
    DEFINE FIELD IF NOT EXISTS created_at ON chunk TYPE datetime DEFAULT time::now();

    DEFINE INDEX IF NOT EXISTS idx_chunk_file      ON chunk FIELDS file;
    DEFINE INDEX IF NOT EXISTS idx_chunk_text_ft    ON chunk FIELDS text FULLTEXT ANALYZER know_analyzer BM25;
    DEFINE INDEX IF NOT EXISTS idx_chunk_embedding  ON chunk FIELDS embedding
        HNSW DIMENSION %d DIST COSINE TYPE F32 EFC 150 M 12 HASHED_VECTOR;

    -- ==========================================================================
    -- WIKI_LINK TABLE
    -- ==========================================================================
    DEFINE TABLE IF NOT EXISTS wiki_link SCHEMAFULL;

    DEFINE FIELD IF NOT EXISTS from_file  ON wiki_link TYPE record<file>;
    DEFINE FIELD IF NOT EXISTS to_file    ON wiki_link TYPE option<record<file>>;
    DEFINE FIELD IF NOT EXISTS raw_target ON wiki_link TYPE string;
    DEFINE FIELD IF NOT EXISTS vault      ON wiki_link TYPE record<vault>;

    DEFINE INDEX IF NOT EXISTS idx_wiki_link_from_file        ON wiki_link FIELDS from_file;
    DEFINE INDEX IF NOT EXISTS idx_wiki_link_to_file          ON wiki_link FIELDS to_file;
    DEFINE INDEX IF NOT EXISTS idx_wiki_link_vault             ON wiki_link FIELDS vault;
    DEFINE INDEX IF NOT EXISTS idx_wiki_link_vault_raw_target  ON wiki_link FIELDS vault, raw_target;

    -- ==========================================================================
    -- EXTERNAL_LINK TABLE
    -- ==========================================================================
    DEFINE TABLE IF NOT EXISTS external_link SCHEMAFULL;

    DEFINE FIELD IF NOT EXISTS from_file  ON external_link TYPE record<file>;
    DEFINE FIELD IF NOT EXISTS vault      ON external_link TYPE record<vault>;
    DEFINE FIELD IF NOT EXISTS hostname   ON external_link TYPE string;
    DEFINE FIELD IF NOT EXISTS url_path   ON external_link TYPE string;
    DEFINE FIELD IF NOT EXISTS full_url   ON external_link TYPE string;
    DEFINE FIELD IF NOT EXISTS link_text  ON external_link TYPE option<string>;

    DEFINE INDEX IF NOT EXISTS idx_external_link_from_file  ON external_link FIELDS from_file;
    DEFINE INDEX IF NOT EXISTS idx_external_link_vault_host ON external_link FIELDS vault, hostname;

    -- ==========================================================================
    -- FILE_RELATION TABLE (RELATION: file -> file)
    -- ==========================================================================
    DEFINE TABLE IF NOT EXISTS file_relation SCHEMAFULL TYPE RELATION FROM file TO file;

    DEFINE FIELD IF NOT EXISTS rel_type   ON file_relation TYPE string;
    DEFINE FIELD IF NOT EXISTS source     ON file_relation TYPE string DEFAULT "manual";
    DEFINE FIELD IF NOT EXISTS created_at ON file_relation TYPE datetime DEFAULT time::now();

    DEFINE INDEX IF NOT EXISTS idx_file_relation_unique ON file_relation FIELDS in, out, rel_type UNIQUE;

    -- ==========================================================================
    -- LABEL TABLE
    -- ==========================================================================
    DEFINE TABLE IF NOT EXISTS label SCHEMAFULL;

    DEFINE FIELD IF NOT EXISTS name       ON label TYPE string ASSERT string::len($value) > 0;
    DEFINE FIELD IF NOT EXISTS vault      ON label TYPE record<vault>;
    DEFINE FIELD IF NOT EXISTS created_at ON label TYPE datetime DEFAULT time::now();

    DEFINE INDEX IF NOT EXISTS idx_label_vault_name ON label FIELDS vault, name UNIQUE;

    -- ==========================================================================
    -- HAS_LABEL RELATION (file -> label)
    -- ==========================================================================
    DEFINE TABLE IF NOT EXISTS has_label SCHEMAFULL TYPE RELATION FROM file TO label;

    DEFINE FIELD IF NOT EXISTS created_at ON has_label TYPE datetime DEFAULT time::now();

    DEFINE INDEX IF NOT EXISTS idx_has_label_unique ON has_label FIELDS in, out UNIQUE;

    -- Cascade: clean up edges when label deleted
    DEFINE EVENT IF NOT EXISTS cascade_delete_label_edges ON label
    WHEN $event = "DELETE" ASYNC RETRY 3 THEN {
        DELETE FROM has_label WHERE out = $before.id
    };

    -- ==========================================================================
    -- API_TOKEN TABLE
    -- ==========================================================================
    DEFINE TABLE IF NOT EXISTS api_token SCHEMAFULL;

    DEFINE FIELD IF NOT EXISTS user         ON api_token TYPE record<user>;
    DEFINE FIELD IF NOT EXISTS token_hash   ON api_token TYPE string;
    DEFINE FIELD IF NOT EXISTS name         ON api_token TYPE string;
    DEFINE FIELD IF NOT EXISTS last_used    ON api_token TYPE option<datetime>;
    DEFINE FIELD IF NOT EXISTS expires_at   ON api_token TYPE option<datetime>;
    DEFINE FIELD IF NOT EXISTS created_at   ON api_token TYPE datetime DEFAULT time::now();

    DEFINE INDEX IF NOT EXISTS idx_api_token_hash ON api_token FIELDS token_hash UNIQUE;

    -- ==========================================================================
    -- VAULT_MEMBER TABLE (user-vault membership with roles)
    -- ==========================================================================
    DEFINE TABLE IF NOT EXISTS vault_member SCHEMAFULL;

    DEFINE FIELD IF NOT EXISTS user       ON vault_member TYPE record<user>;
    DEFINE FIELD IF NOT EXISTS vault      ON vault_member TYPE record<vault>;
    DEFINE FIELD IF NOT EXISTS role       ON vault_member TYPE string ASSERT $value IN ["read", "write", "admin"];
    DEFINE FIELD IF NOT EXISTS created_at ON vault_member TYPE datetime DEFAULT time::now();

    DEFINE INDEX IF NOT EXISTS idx_vault_member_user_vault ON vault_member FIELDS user, vault UNIQUE;
    DEFINE INDEX IF NOT EXISTS idx_vault_member_vault      ON vault_member FIELDS vault;

    -- Cascade: delete vault_member when vault deleted
    DEFINE EVENT IF NOT EXISTS cascade_delete_vault_members ON vault
    WHEN $event = "DELETE" ASYNC RETRY 3 THEN {
        DELETE FROM vault_member WHERE vault = $before.id
    };

    -- ==========================================================================
    -- SHARE_LINK TABLE (read-only share links for files/folders)
    -- ==========================================================================
    DEFINE TABLE IF NOT EXISTS share_link SCHEMAFULL;

    DEFINE FIELD IF NOT EXISTS vault      ON share_link TYPE record<vault>;
    DEFINE FIELD IF NOT EXISTS token_hash ON share_link TYPE string;
    DEFINE FIELD IF NOT EXISTS path       ON share_link TYPE string;
    DEFINE FIELD IF NOT EXISTS is_folder  ON share_link TYPE bool DEFAULT false;
    DEFINE FIELD IF NOT EXISTS created_by ON share_link TYPE record<user>;
    DEFINE FIELD IF NOT EXISTS expires_at ON share_link TYPE option<datetime>;
    DEFINE FIELD IF NOT EXISTS created_at ON share_link TYPE datetime DEFAULT time::now();

    DEFINE INDEX IF NOT EXISTS idx_share_link_hash ON share_link FIELDS token_hash UNIQUE;

    -- Cascade: delete labels when vault deleted
    DEFINE EVENT IF NOT EXISTS cascade_delete_vault_labels ON vault
    WHEN $event = "DELETE" ASYNC RETRY 3 THEN {
        DELETE FROM label WHERE vault = $before.id
    };

    -- Cascade: delete share_links when vault deleted
    DEFINE EVENT IF NOT EXISTS cascade_delete_vault_share_links ON vault
    WHEN $event = "DELETE" ASYNC RETRY 3 THEN {
        DELETE FROM share_link WHERE vault = $before.id
    };

    -- ==========================================================================
    -- CONVERSATION TABLE
    -- ==========================================================================
    DEFINE TABLE IF NOT EXISTS conversation SCHEMAFULL;

    DEFINE FIELD IF NOT EXISTS vault      ON conversation TYPE record<vault>;
    DEFINE FIELD IF NOT EXISTS user       ON conversation TYPE record<user>;
    DEFINE FIELD IF NOT EXISTS title      ON conversation TYPE string DEFAULT "New conversation";
    DEFINE FIELD IF NOT EXISTS created_at    ON conversation TYPE datetime DEFAULT time::now();
    DEFINE FIELD IF NOT EXISTS updated_at    ON conversation TYPE datetime VALUE time::now();
    DEFINE FIELD IF NOT EXISTS token_input   ON conversation TYPE int DEFAULT 0;
    DEFINE FIELD IF NOT EXISTS token_output  ON conversation TYPE int DEFAULT 0;
    DEFINE FIELD IF NOT EXISTS bg_status       ON conversation TYPE option<string>;
    DEFINE FIELD IF NOT EXISTS bg_error        ON conversation TYPE option<string>;
    DEFINE FIELD IF NOT EXISTS bg_started_at   ON conversation TYPE option<datetime>;
    DEFINE FIELD IF NOT EXISTS bg_completed_at ON conversation TYPE option<datetime>;

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

    -- ==========================================================================
    -- FILE_TOMBSTONE TABLE (tracks deleted files for sync)
    -- ==========================================================================
    DEFINE TABLE IF NOT EXISTS file_tombstone SCHEMAFULL;

    DEFINE FIELD IF NOT EXISTS vault      ON file_tombstone TYPE record<vault>;
    DEFINE FIELD IF NOT EXISTS file_id    ON file_tombstone TYPE string;
    DEFINE FIELD IF NOT EXISTS path       ON file_tombstone TYPE string;
    DEFINE FIELD IF NOT EXISTS deleted_at ON file_tombstone TYPE datetime DEFAULT time::now();

    DEFINE INDEX IF NOT EXISTS idx_tombstone_vault_since ON file_tombstone FIELDS vault, deleted_at;

    -- ==========================================================================
    -- TASK TABLE (extracted from markdown file checkboxes)
    -- ==========================================================================
    DEFINE TABLE IF NOT EXISTS task SCHEMAFULL;

    DEFINE FIELD IF NOT EXISTS file         ON task TYPE record<file>;
    DEFINE FIELD IF NOT EXISTS vault        ON task TYPE record<vault>;
    DEFINE FIELD IF NOT EXISTS status       ON task TYPE string ASSERT $value IN ["open", "done"];
    DEFINE FIELD IF NOT EXISTS raw_line     ON task TYPE string;
    DEFINE FIELD IF NOT EXISTS text         ON task TYPE string;
    DEFINE FIELD IF NOT EXISTS labels       ON task TYPE array<string> DEFAULT [];
    DEFINE FIELD IF NOT EXISTS due_date     ON task TYPE option<string>;
    DEFINE FIELD IF NOT EXISTS line_number  ON task TYPE int;
    DEFINE FIELD IF NOT EXISTS heading_path ON task TYPE option<string>;
    DEFINE FIELD IF NOT EXISTS content_hash ON task TYPE string;
    DEFINE FIELD IF NOT EXISTS created_at   ON task TYPE datetime DEFAULT time::now();
    DEFINE FIELD IF NOT EXISTS updated_at   ON task TYPE datetime VALUE time::now();

    DEFINE INDEX IF NOT EXISTS idx_task_file          ON task FIELDS file;
    DEFINE INDEX IF NOT EXISTS idx_task_vault_status   ON task FIELDS vault, status;
    DEFINE INDEX IF NOT EXISTS idx_task_vault_due_date ON task FIELDS vault, due_date;
    DEFINE INDEX IF NOT EXISTS idx_task_vault_labels   ON task FIELDS vault, labels;

    -- ==========================================================================
    -- REMOTE TABLE (federation: connections to other know servers)
    -- ==========================================================================
    DEFINE TABLE IF NOT EXISTS remote SCHEMAFULL;

    DEFINE FIELD IF NOT EXISTS name       ON remote TYPE string;
    DEFINE FIELD IF NOT EXISTS url        ON remote TYPE string;
    DEFINE FIELD IF NOT EXISTS token      ON remote TYPE string;
    DEFINE FIELD IF NOT EXISTS created_by ON remote TYPE record<user>;
    DEFINE FIELD IF NOT EXISTS created_at ON remote TYPE datetime DEFAULT time::now();
    DEFINE FIELD IF NOT EXISTS updated_at ON remote TYPE datetime VALUE time::now();

    DEFINE INDEX IF NOT EXISTS idx_remote_name ON remote FIELDS name UNIQUE;

    -- ==========================================================================
    -- AGENT_CHECKPOINT TABLE (eino interrupt/resume checkpoint persistence)
    -- ==========================================================================
    DEFINE TABLE IF NOT EXISTS agent_checkpoint SCHEMAFULL;

    DEFINE FIELD IF NOT EXISTS data       ON agent_checkpoint TYPE bytes;
    DEFINE FIELD IF NOT EXISTS updated_at ON agent_checkpoint TYPE datetime VALUE time::now();

    -- ==========================================================================
    -- PIPELINE_JOB TABLE (unified job queue for parse/chunk/embed/transcribe)
    -- ==========================================================================
    DEFINE TABLE IF NOT EXISTS pipeline_job SCHEMAFULL;

    DEFINE FIELD IF NOT EXISTS file         ON pipeline_job TYPE record<file>;
    DEFINE FIELD IF NOT EXISTS type         ON pipeline_job TYPE string;
    DEFINE FIELD IF NOT EXISTS status       ON pipeline_job TYPE string DEFAULT "pending";
    DEFINE FIELD IF NOT EXISTS priority     ON pipeline_job TYPE int DEFAULT 0;
    DEFINE FIELD IF NOT EXISTS attempt      ON pipeline_job TYPE int DEFAULT 0;
    DEFINE FIELD IF NOT EXISTS max_attempts ON pipeline_job TYPE int DEFAULT 5;
    DEFINE FIELD IF NOT EXISTS run_after    ON pipeline_job TYPE option<datetime>;
    DEFINE FIELD IF NOT EXISTS error        ON pipeline_job TYPE option<string>;
    DEFINE FIELD IF NOT EXISTS created_at   ON pipeline_job TYPE datetime DEFAULT time::now();
    DEFINE FIELD IF NOT EXISTS updated_at   ON pipeline_job TYPE datetime VALUE time::now();

    DEFINE INDEX IF NOT EXISTS idx_job_pending ON pipeline_job
        FIELDS status, run_after, priority, created_at;
`, dimension)
}
