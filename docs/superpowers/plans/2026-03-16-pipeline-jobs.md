# Pipeline Job Table — Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace ad-hoc scheduling fields (`processed`, `embed_at`, `transcribe_at`) and three separate workers with a single `pipeline_job` table and generic PipelineWorker.

**Architecture:** A `pipeline_job` SurrealDB table serves as a job queue. One `PipelineWorker` goroutine claims jobs via atomic UPDATE subquery, dispatches to registered handler functions by job type. Handlers create next-step jobs on completion. Event bus provides instant wake-up between steps.

**Tech Stack:** Go, SurrealDB, existing WorkerLoop + event bus infrastructure

**Spec:** `docs/superpowers/specs/2026-03-16-pipeline-job-design.md`

**Prerequisite:** Blob storage abstraction (plan: `2026-03-16-blob-storage.md`) must be implemented first — the `transcribe` handler reads blobs from the store.

---

## File Structure

| File | Responsibility |
|------|---------------|
| `internal/models/pipeline_job.go` | PipelineJob struct |
| `internal/db/queries_job.go` | Job CRUD: create, claim, complete, fail, retry, cancel |
| `internal/db/schema.go` | Add pipeline_job table, remove processed/embed_at/transcribe_at |
| `internal/pipeline/worker.go` | Generic PipelineWorker with handler registry |
| `internal/pipeline/worker_test.go` | Worker unit tests |
| `internal/pipeline/handlers.go` | Handler functions: parse, chunk, transcribe, embed |
| `internal/pipeline/handlers_test.go` | Handler unit tests |
| `internal/file/service.go` | Create() creates jobs; remove ProcessFile/EmbedPendingChunks/TranscribePendingFiles |
| `internal/file/process_worker.go` | **Delete** |
| `internal/file/worker.go` | **Delete** |
| `internal/file/transcription_worker.go` | **Delete** |
| `internal/file/worker_test.go` | **Delete** |
| `internal/config/config.go` | Replace 6 worker config fields with 2 |
| `internal/server/bootstrap.go` | Replace 3 worker lifecycles with 1 |

---

### Task 1: PipelineJob Model + DB Schema

**Files:**
- Create: `internal/models/pipeline_job.go`
- Modify: `internal/db/schema.go`

- [ ] **Step 1: Create PipelineJob model**

```go
// internal/models/pipeline_job.go
package models

import (
	"time"
	surrealmodels "github.com/surrealdb/surrealdb.go/pkg/models"
)

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

- [ ] **Step 2: Add pipeline_job table to schema**

In `internal/db/schema.go`, add the table definition from the spec. Include the composite index on `status, run_after, priority, created_at`.

- [ ] **Step 3: Remove old scheduling fields from schema**

- Remove `processed` bool from file table
- Remove `embed_at` from chunk table
- Remove `transcribe_at` from file table

- [ ] **Step 4: Update Go models**

- Remove `Processed bool` from `models.File`
- Remove `TranscribeAt *time.Time` from `models.File`
- Remove `EmbedAt *time.Time` from `models.Chunk` and `models.ChunkInput`

- [ ] **Step 5: Build to check what breaks**

Run: `go build -buildvcs=false ./...` — catalog all compilation errors. These are the consumer sites to update in later tasks.

- [ ] **Step 6: Commit**

```
refactor: add pipeline_job table, remove processed/embed_at/transcribe_at
```

---

### Task 2: DB Queries for Jobs

**Files:**
- Create: `internal/db/queries_job.go`

- [ ] **Step 1: Implement job queries**

```go
// internal/db/queries_job.go
package db

func (c *Client) CreateJob(ctx context.Context, fileID, jobType string, priority int) error
// INSERT INTO pipeline_job { file: type::record("file", $file_id), type: $type, priority: $priority }

func (c *Client) ClaimJobs(ctx context.Context, limit int) ([]models.PipelineJob, error)
// UPDATE (SELECT * FROM pipeline_job WHERE status = 'pending' AND (run_after IS NONE OR run_after <= time::now()) ORDER BY priority DESC, created_at ASC LIMIT $limit) SET status = 'running' RETURN BEFORE

func (c *Client) CompleteJob(ctx context.Context, jobID string) error
// UPDATE type::record("pipeline_job", $id) SET status = 'done'

func (c *Client) FailJob(ctx context.Context, jobID string, errMsg string) error
// UPDATE type::record("pipeline_job", $id) SET status = 'failed', error = $error

func (c *Client) RetryJob(ctx context.Context, jobID string, runAfter time.Time) error
// UPDATE type::record("pipeline_job", $id) SET status = 'pending', attempt = attempt + 1, run_after = $run_after

func (c *Client) CancelJobsForFile(ctx context.Context, fileID string) error
// UPDATE pipeline_job SET status = 'done' WHERE file = type::record("file", $file_id) AND status IN ['pending', 'running']

func (c *Client) ListFailedJobs(ctx context.Context, limit int) ([]models.PipelineJob, error)
// SELECT * FROM pipeline_job WHERE status = 'failed' ORDER BY updated_at DESC LIMIT $limit
```

All methods use `defer c.logOp(ctx, "pipeline_job.operation", time.Now())`.

- [ ] **Step 2: Build to verify**

Run: `go build -buildvcs=false ./...`

- [ ] **Step 3: Commit**

```
feat: add pipeline job DB queries (create, claim, complete, fail, retry)
```

---

### Task 3: Generic PipelineWorker

**Files:**
- Create: `internal/pipeline/worker.go`
- Create: `internal/pipeline/worker_test.go`

- [ ] **Step 1: Implement PipelineWorker**

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
// Uses file.NewWorkerLoop with file.eventNotify(bus, "job.created")
```

Note: `WorkerLoop` and `eventNotify` are currently in `internal/file/`. Either:
- Move them to a shared package (e.g., `internal/worker/`)
- Or keep them in `internal/file/` and import from pipeline

Choose whichever is cleaner. Moving to `internal/worker/` is recommended since `pipeline` shouldn't depend on `file`.

The `tick` method:
1. `db.ClaimJobs(ctx, w.batch)`
2. For each job: look up handler by `job.Type`
3. Call handler
4. On success: `db.CompleteJob(ctx, jobID)`
5. On failure: if `job.Attempt+1 < job.MaxAttempts` → `db.RetryJob(ctx, jobID, time.Now().Add(30*time.Second))`, else `db.FailJob(ctx, jobID, err.Error())`

- [ ] **Step 2: Write tests**

Test with mock handlers:
- `TestWorker_DispatchesToHandler` — register handler, create job, verify handler called
- `TestWorker_RetriesOnFailure` — handler returns error, verify job retried
- `TestWorker_FailsAfterMaxAttempts` — verify job marked failed after max retries
- `TestWorker_UnknownTypeSkipped` — job with unregistered type logged and skipped

- [ ] **Step 3: Run tests**

Run: `go test ./internal/pipeline/...`

- [ ] **Step 4: Commit**

```
feat: add generic PipelineWorker with handler registry
```

---

### Task 4: Move WorkerLoop to Shared Package

**Files:**
- Create: `internal/worker/loop.go` (move from `internal/file/worker_loop.go`)
- Create: `internal/worker/event.go` (move `eventNotify` from `internal/file/worker_loop.go`)
- Modify: `internal/file/worker_loop.go` — delete or re-export
- Modify: `internal/pipeline/worker.go` — import from `internal/worker/`

- [ ] **Step 1: Create internal/worker/ package**

Move `WorkerLoop`, `NewWorkerLoop`, and `eventNotify` from `internal/file/worker_loop.go` to `internal/worker/loop.go`. Update package name.

- [ ] **Step 2: Update all references**

- `internal/file/process_worker.go` (will be deleted later, but fix for now)
- `internal/file/worker.go` (will be deleted later)
- `internal/pipeline/worker.go`

- [ ] **Step 3: Build and test**

Run: `go build -buildvcs=false ./...` and `go test ./internal/worker/...`

- [ ] **Step 4: Commit**

```
refactor: move WorkerLoop to shared internal/worker package
```

---

### Task 5: Pipeline Handlers — Parse + Chunk

**Files:**
- Create: `internal/pipeline/handlers.go`

- [ ] **Step 1: Implement parse handler**

Extract logic from current `ProcessFile` for text files:

```go
func ParseHandler(fileSvc *file.Service, db *db.Client, bus *event.Bus) Handler {
	return func(ctx context.Context, job models.PipelineJob) error {
		fileID, _ := models.RecordIDString(job.File)
		doc, err := db.GetFileByID(ctx, fileID)
		// Parse markdown, process wiki-links, relations, tasks, labels
		// (call existing service methods that currently live in ProcessFile)
		// Create next job:
		db.CreateJob(ctx, fileID, "chunk", 0)
		bus.Publish(event.ChangeEvent{Type: "job.created"})
		return nil
	}
}
```

The parse handler needs access to the same service methods currently called by `ProcessFile`. These methods (`processWikiLinks`, `resolveDanglingForFile`, `processRelatesTo`, `syncTasks`, `SyncFileLabels`) are on `file.Service` — the handler calls them via the service.

Consider adding a `file.Service.Parse(ctx, fileID)` method that wraps these calls, so the handler stays thin.

- [ ] **Step 2: Implement chunk handler**

Extract `syncChunks` logic:

```go
func ChunkHandler(fileSvc *file.Service, db *db.Client, embedder *llm.Embedder, bus *event.Bus) Handler {
	return func(ctx context.Context, job models.PipelineJob) error {
		fileID, _ := models.RecordIDString(job.File)
		// Call service method to sync chunks
		// If embedder configured and new chunks created:
		db.CreateJob(ctx, fileID, "embed", 0)
		bus.Publish(event.ChangeEvent{Type: "job.created"})
		return nil
	}
}
```

- [ ] **Step 3: Build**

Run: `go build -buildvcs=false ./...`

- [ ] **Step 4: Commit**

```
feat: add parse and chunk pipeline handlers
```

---

### Task 6: Pipeline Handlers — Transcribe + Embed

**Files:**
- Modify: `internal/pipeline/handlers.go`

- [ ] **Step 1: Implement transcribe handler**

Extract `transcribeFile` logic:

```go
func TranscribeHandler(fileSvc *file.Service, transcriber stt.Transcriber, blobStore blob.Store, db *db.Client, bus *event.Bus) Handler {
	return func(ctx context.Context, job models.PipelineJob) error {
		fileID, _ := models.RecordIDString(job.File)
		// Fetch file metadata (not blob)
		// Get blob from blobStore
		// Transcribe (with ffmpeg split if needed)
		// Group segments, create chunks
		// Update file.Content
		// Create embed job
		db.CreateJob(ctx, fileID, "embed", 0)
		bus.Publish(event.ChangeEvent{Type: "job.created"})
		return nil
	}
}
```

- [ ] **Step 2: Implement embed handler**

Extract `EmbedPendingChunks` logic, scoped to one file:

```go
func EmbedHandler(fileSvc *file.Service, db *db.Client) Handler {
	return func(ctx context.Context, job models.PipelineJob) error {
		fileID, _ := models.RecordIDString(job.File)
		// Fetch all chunks for file without embeddings
		// Build contextual embedding text
		// Batch embed
		// Store embeddings
		return nil // terminal step, no next job
	}
}
```

- [ ] **Step 3: Build and test**

Run: `go build -buildvcs=false ./...`

- [ ] **Step 4: Commit**

```
feat: add transcribe and embed pipeline handlers
```

---

### Task 7: Update File Service — Create Jobs Instead of ProcessFile

**Files:**
- Modify: `internal/file/service.go`

- [ ] **Step 1: Update Create() to create jobs**

Replace `processed = false` logic with job creation:

```go
// After storing file in DB:
if isBinary && models.IsAudioFile(input.Path) {
	if err := s.db.CreateJob(ctx, fileID, "transcribe", 0); err != nil {
		return nil, fmt.Errorf("create transcribe job: %w", err)
	}
} else if !isBinary {
	if err := s.db.CreateJob(ctx, fileID, "parse", 0); err != nil {
		return nil, fmt.Errorf("create parse job: %w", err)
	}
}
s.bus.Publish(event.ChangeEvent{Type: "job.created"})
```

- [ ] **Step 2: Handle file updates**

On file update (content changed), cancel existing jobs and create new ones:

```go
s.db.CancelJobsForFile(ctx, fileID)
s.db.CreateJob(ctx, fileID, "parse", 0) // or "transcribe" for audio
```

- [ ] **Step 3: Remove ProcessFile, EmbedPendingChunks, TranscribePendingFiles**

These methods are now replaced by pipeline handlers. Remove them from `service.go`. Keep helper methods they call (`syncChunks`, `processWikiLinks`, etc.) — the handlers use them.

Also remove: `ProcessAllPending` (used in tests/CLI — replace with a `ProcessAll` that creates jobs and waits).

- [ ] **Step 4: Build**

Run: `go build -buildvcs=false ./...`

- [ ] **Step 5: Commit**

```
refactor: create pipeline jobs in file service instead of inline processing
```

---

### Task 8: Delete Old Workers + Update Bootstrap

**Files:**
- Delete: `internal/file/process_worker.go`
- Delete: `internal/file/worker.go`
- Delete: `internal/file/transcription_worker.go`
- Delete: `internal/file/worker_test.go`
- Modify: `internal/config/config.go`
- Modify: `internal/server/bootstrap.go`

- [ ] **Step 1: Delete old worker files**

Remove:
- `internal/file/process_worker.go`
- `internal/file/worker.go`
- `internal/file/transcription_worker.go`
- `internal/file/worker_test.go`

- [ ] **Step 2: Update config**

Replace 6 worker config fields with 2:
```go
// Pipeline worker settings
PipelineWorkerInterval int // KNOW_PIPELINE_WORKER_INTERVAL (default: 5)
PipelineWorkerBatch    int // KNOW_PIPELINE_WORKER_BATCH (default: 10)
```

Remove:
- `EmbedWorkerInterval`, `EmbedWorkerBatch`
- `ProcessingWorkerInterval`, `ProcessingWorkerBatch`
- `TranscriptionWorkerInterval`, `TranscriptionWorkerBatch`

- [ ] **Step 3: Update bootstrap.go**

Replace 3 worker lifecycles with 1:

```go
pipelineWorker := pipeline.NewWorker(dbClient, bus, interval, batch)
pipelineWorker.Register("parse", pipeline.ParseHandler(fileSvc, dbClient, bus))
pipelineWorker.Register("chunk", pipeline.ChunkHandler(fileSvc, dbClient, embedder, bus))
pipelineWorker.Register("embed", pipeline.EmbedHandler(fileSvc, dbClient))
if transcriber != nil {
	pipelineWorker.Register("transcribe", pipeline.TranscribeHandler(fileSvc, transcriber, blobStore, dbClient, bus))
}
```

Replace `syncEmbeddingWorker`, `startProcessingWorker`, `syncTranscriptionWorker` with single `syncPipelineWorker`.

Remove 3 pairs of cancel/done fields, replace with 1 pair.

Update `Close()` and `ReloadLLM()` accordingly.

- [ ] **Step 4: Build and test**

Run: `go build -buildvcs=false ./...` and `just test`

- [ ] **Step 5: Commit**

```
refactor: replace three workers with single PipelineWorker
```

---

### Task 9: Remove Old DB Queries + Clean Up

**Files:**
- Modify: `internal/db/queries_file.go`
- Modify: `internal/db/queries_chunk.go`

- [ ] **Step 1: Remove obsolete queries**

From `queries_file.go`:
- Remove `ListUnprocessedFiles`
- Remove `MarkFileProcessed`
- Remove `ScheduleFileTranscription`
- Remove `ClaimFilesForTranscription`
- Remove `UpdateFileTranscript` (if transcript update moves to handler)
- Remove `RescheduleFileTranscription`

From `queries_chunk.go`:
- Remove `ClaimChunksForEmbedding`
- Remove `RescheduleChunkEmbedding` (if it exists)

- [ ] **Step 2: Build and test**

Run: `go build -buildvcs=false ./...` and `just test`

- [ ] **Step 3: Commit**

```
refactor: remove obsolete worker scheduling queries
```

---

### Task 10: Final Verification

- [ ] **Step 1: Full build**

Run: `just build`

- [ ] **Step 2: Full tests**

Run: `just test`

- [ ] **Step 3: Manual end-to-end — text file**

```bash
just bootstrap && just dev
just run cp ./docs / --vault default
# Verify: SELECT * FROM pipeline_job shows parse → chunk → embed chain
# Verify: all jobs status='done'
# Verify: chunks have embeddings
```

- [ ] **Step 4: Manual end-to-end — audio file**

```bash
KNOW_STT_PROVIDER=openai just dev
just run cp ~/test-audio.mp3 /test/ --vault default
# Verify: SELECT * FROM pipeline_job shows transcribe → embed chain
# Verify: file.Content has transcript
# Verify: search finds spoken words
```

- [ ] **Step 5: Verify retry behavior**

Kill server mid-transcription, restart. Verify the job retries (attempt incremented, eventually completes).

- [ ] **Step 6: Verify failed jobs**

```sql
SELECT * FROM pipeline_job WHERE status = 'failed';
SELECT type, status, count() FROM pipeline_job GROUP BY type, status;
```
