# Pipeline Job Table

## Context

The current ingestion pipeline uses three ad-hoc scheduling mechanisms:
- `processed` bool on file — drives the ProcessingWorker
- `embed_at` datetime on chunk — drives the EmbeddingWorker
- `transcribe_at` datetime on file — drives the TranscriptionWorker

Each new processing step requires a new scheduling field and a new worker with its own lifecycle management in bootstrap.go. This doesn't scale as we add more file types (PDF text extraction, image OCR, etc.).

This spec replaces all three with a single `pipeline_job` table and a generic PipelineWorker that dispatches to registered handlers.

## Design Decisions

- **One job per processing step** — `parse`, `chunk`, `transcribe`, `embed`. Fine-grained, independently retryable.
- **Per-file embedding** — one `embed` job per file (handler batch-embeds all chunks for that file).
- **One generic PipelineWorker** — single goroutine, claims jobs by priority, dispatches to registered handlers.
- **Poll + event-driven** — WorkerLoop polls on interval AND wakes on `job.created` events for instant chaining.
- **Handlers create next jobs** — on completion, each handler creates the next job in the chain. No central orchestrator.

## Job Table Schema

```sql
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
```

Status values: `pending`, `running`, `done`, `failed`.

## Pipeline Per File Type

**Text files (markdown):**
```
file.created → parse → chunk → embed
```

**Audio files:**
```
file.created → transcribe → chunk → embed
```

**Images/PDFs (future):**
```
file.created → extract_text → chunk → embed
```

## Job Handlers

| Type | Input | Does | Creates Next |
|------|-------|------|-------------|
| `parse` | file ID | frontmatter, wiki-links, relations, tasks, labels | `chunk` job |
| `chunk` | file ID | splits content into text chunks, stores in DB | `embed` job |
| `transcribe` | file ID | calls STT provider, stores transcript in file.Content, creates chunks | `embed` job |
| `embed` | file ID | batch-embeds all un-embedded chunks for the file | nothing |

### Handler Details

**`parse`** — Extracts from current `ProcessFile` for text files:
- Parse markdown (frontmatter, title, labels, metadata)
- Process wiki-links (extract, resolve, resolve dangling)
- Process `relates_to` from frontmatter
- Sync tasks (checkbox extraction)
- Sync labels
- On completion: creates `chunk` job

**`chunk`** — Extracts from current `syncChunks`:
- Parse markdown into chunks with heading paths
- Smart diff against existing chunks (preserve embeddings for unchanged content)
- Create/update/delete chunks
- On completion: creates `embed` job (only if embedder is configured and chunks need embedding)

**`transcribe`** — Current `transcribeFile` logic:
- Fetch file blob from blob store
- Call STT provider (split via ffmpeg if >25MB)
- Group segments into time-window chunks via `GroupSegments`
- Store chunks in DB
- Update file.Content with full transcript
- On completion: creates `embed` job

**`embed`** — Current `EmbedPendingChunks` logic, scoped to one file:
- Fetch all chunks for file that have no embedding
- Build contextual embedding text (file title + section path + content)
- Batch embed via embedder
- Store embeddings on chunks
- On completion: nothing (terminal step)

## Generic PipelineWorker

```go
// internal/pipeline/worker.go
package pipeline

type Handler func(ctx context.Context, job models.PipelineJob) error

type Worker struct {
    db       *db.Client
    handlers map[string]Handler
    bus      *event.Bus
    interval time.Duration
    batch    int
}

func NewWorker(db *db.Client, bus *event.Bus, interval time.Duration, batch int) *Worker

func (w *Worker) Register(jobType string, handler Handler)

func (w *Worker) Run(ctx context.Context)
// Uses WorkerLoop with eventNotify(bus, "job.created")
// tick() claims jobs, dispatches to handlers
```

### Claiming Pattern

Same atomic subquery-UPDATE as `ClaimChunksForEmbedding`:

```sql
UPDATE (
    SELECT * FROM pipeline_job
    WHERE status = 'pending'
      AND (run_after IS NONE OR run_after <= time::now())
    ORDER BY priority DESC, created_at ASC
    LIMIT $limit
)
SET status = 'running'
RETURN BEFORE
```

### Completion Flow

```go
func (w *Worker) tick(ctx context.Context) {
    jobs := w.db.ClaimJobs(ctx, w.batch)
    for _, job := range jobs {
        handler := w.handlers[job.Type]
        if err := handler(ctx, job); err != nil {
            w.handleFailure(ctx, job, err)
            continue
        }
        w.db.CompleteJob(ctx, jobID)
    }
}
```

On failure:
- Increment `attempt`
- If `attempt < max_attempts`: set `status='pending'`, `run_after = now()+30s`
- If `attempt >= max_attempts`: set `status='failed'`, `error = err.Error()`

### Event-Driven Wake

When a handler creates the next job, it publishes on the event bus:
```go
bus.Publish(event.ChangeEvent{Type: "job.created", ...})
```

The WorkerLoop wakes immediately to process the next step. Full chain `parse → chunk → embed` fires in rapid succession without waiting for poll intervals.

## Job Model

```go
// internal/models/pipeline_job.go
type PipelineJob struct {
    ID          surrealmodels.RecordID `json:"id"`
    File        surrealmodels.RecordID `json:"file"`
    Type        string                 `json:"type"`
    Status      string                 `json:"status"`
    Priority    int                    `json:"priority"`
    Attempt     int                    `json:"attempt"`
    MaxAttempts int                    `json:"max_attempts"`
    RunAfter    *time.Time             `json:"run_after,omitempty"`
    Error       *string                `json:"error,omitempty"`
    CreatedAt   time.Time              `json:"created_at"`
    UpdatedAt   time.Time              `json:"updated_at"`
}
```

## Job Creation

In `file.Service.Create()`, after storing the file metadata:

```go
if isBinary && models.IsAudioFile(input.Path) {
    db.CreateJob(ctx, fileID, "transcribe")
} else if !isBinary {
    db.CreateJob(ctx, fileID, "parse")
}
bus.Publish(event.ChangeEvent{Type: "job.created", ...})
```

On file update (content changed): create new `parse` or `transcribe` job. Existing pending/running jobs for the file are cancelled (set `status='done'` — new job supersedes).

## What Gets Removed

| Removed | Replaced By |
|---------|-------------|
| `processed` bool on file table | `pipeline_job` status |
| `embed_at` datetime on chunk table | `embed` job |
| `transcribe_at` datetime on file table | `transcribe` job |
| `ProcessingWorker` | PipelineWorker + `parse`/`chunk` handlers |
| `EmbeddingWorker` | PipelineWorker + `embed` handler |
| `TranscriptionWorker` | PipelineWorker + `transcribe` handler |
| `ListUnprocessedFiles` query | `ClaimJobs` query |
| `ClaimChunksForEmbedding` query | `ClaimJobs` query |
| `ClaimFilesForTranscription` query | `ClaimJobs` query |
| `syncEmbeddingWorker` in bootstrap | single `syncPipelineWorker` |
| `startProcessingWorker` in bootstrap | (same) |
| `syncTranscriptionWorker` in bootstrap | (same) |
| Worker cancel/done fields (3 pairs) | single cancel/done pair |

## DB Queries

```go
// internal/db/queries_job.go

func (c *Client) CreateJob(ctx, fileID, jobType string, priority int) error
func (c *Client) ClaimJobs(ctx, limit int) ([]models.PipelineJob, error)
func (c *Client) CompleteJob(ctx, jobID string) error
func (c *Client) FailJob(ctx, jobID string, err string) error
func (c *Client) RetryJob(ctx, jobID string, runAfter time.Time) error
func (c *Client) CancelJobsForFile(ctx, fileID string) error
func (c *Client) ListFailedJobs(ctx, limit int) ([]models.PipelineJob, error)
```

## Configuration

```
KNOW_PIPELINE_WORKER_INTERVAL=5    (seconds, default: 5)
KNOW_PIPELINE_WORKER_BATCH=10      (jobs per tick, default: 10)
```

Replaces:
- `KNOW_EMBED_WORKER_INTERVAL` / `KNOW_EMBED_WORKER_BATCH`
- `KNOW_PROCESSING_WORKER_INTERVAL` / `KNOW_PROCESSING_WORKER_BATCH`
- `KNOW_TRANSCRIPTION_WORKER_INTERVAL` / `KNOW_TRANSCRIPTION_WORKER_BATCH`

## Bootstrap Wiring

In `internal/server/bootstrap.go`:

```go
pipelineWorker := pipeline.NewWorker(dbClient, bus, interval, batch)
pipelineWorker.Register("parse", handlers.ParseHandler(fileSvc))
pipelineWorker.Register("chunk", handlers.ChunkHandler(fileSvc))
pipelineWorker.Register("embed", handlers.EmbedHandler(fileSvc, embedder))
if transcriber != nil {
    pipelineWorker.Register("transcribe", handlers.TranscribeHandler(fileSvc, transcriber, blobStore))
}
go pipelineWorker.Run(ctx)
```

Single worker goroutine replaces three.

## Files to Create/Modify

| File | Action |
|------|--------|
| `internal/models/pipeline_job.go` | **New** — PipelineJob struct |
| `internal/db/queries_job.go` | **New** — job CRUD + claim queries |
| `internal/db/schema.go` | **Modify** — add pipeline_job table, remove processed/embed_at/transcribe_at |
| `internal/pipeline/worker.go` | **New** — generic PipelineWorker (replaces current worker_loop usage) |
| `internal/pipeline/handlers.go` | **New** — parse, chunk, transcribe, embed handlers |
| `internal/file/service.go` | **Modify** — Create() creates jobs instead of setting processed=false; remove ProcessFile/EmbedPendingChunks/TranscribePendingFiles |
| `internal/file/process_worker.go` | **Delete** |
| `internal/file/worker.go` | **Delete** |
| `internal/file/transcription_worker.go` | **Delete** |
| `internal/file/worker_test.go` | **Delete** |
| `internal/config/config.go` | **Modify** — replace 6 worker config fields with 2 |
| `internal/server/bootstrap.go` | **Modify** — replace 3 worker lifecycles with 1 |

## Observability

Jobs are queryable — failed jobs show what went wrong:

```sql
SELECT * FROM pipeline_job WHERE status = 'failed' ORDER BY updated_at DESC;
SELECT type, count() FROM pipeline_job GROUP BY type, status;
```

Future: expose via REST API (`GET /api/jobs?status=failed`).

## Verification

1. `just build` — compiles
2. `just test` — all tests pass
3. Manual: ingest text file → verify parse → chunk → embed jobs chain
4. Manual: ingest audio file → verify transcribe → chunk → embed jobs chain
5. Verify failed jobs: kill server mid-transcription → restart → job retries
6. Verify `SELECT * FROM pipeline_job` shows job history
