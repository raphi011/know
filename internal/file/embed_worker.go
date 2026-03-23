package file

import (
	"context"
	"time"

	"github.com/raphi011/know/internal/db"
	"github.com/raphi011/know/internal/event"
	"github.com/raphi011/know/internal/logutil"
	"github.com/raphi011/know/internal/metrics"
	"github.com/raphi011/know/internal/models"
	"github.com/raphi011/know/internal/worker"
)

// EmbedWorker periodically sweeps for unembedded chunks across all files
// and embeds them in batches. This replaces per-file embed pipeline jobs.
type EmbedWorker struct {
	svc      *Service
	db       *db.Client
	bus      *event.Bus
	metrics  *metrics.Metrics
	interval time.Duration
	batch    int
}

// NewEmbedWorker creates a new EmbedWorker.
func NewEmbedWorker(svc *Service, dbClient *db.Client, bus *event.Bus, interval time.Duration, batch int, m *metrics.Metrics) *EmbedWorker {
	if interval <= 0 {
		panic("file.EmbedWorker: interval must be positive")
	}
	if batch <= 0 {
		panic("file.EmbedWorker: batch must be positive")
	}
	return &EmbedWorker{
		svc:      svc,
		db:       dbClient,
		bus:      bus,
		metrics:  m,
		interval: interval,
		batch:    batch,
	}
}

// Run starts the embed worker loop. It blocks until ctx is cancelled.
func (w *EmbedWorker) Run(ctx context.Context) {
	notify, unsub := worker.EventNotify(w.bus, "file.processed")
	defer unsub()
	loop := worker.NewWorkerLoop("embed worker", w.interval, w.tick, notify)
	loop.Run(ctx)
}

func (w *EmbedWorker) tick(ctx context.Context) {
	logger := logutil.FromCtx(ctx)

	embedder := w.svc.getEmbedder()
	mmEmbedder := w.svc.getMultimodalEmbedder()
	if embedder == nil && mmEmbedder == nil {
		return
	}

	chunks, err := w.db.GetUnembeddedChunksBatch(ctx, w.batch)
	if err != nil {
		logger.Error("embed worker: get unembedded chunks", "error", err)
		return
	}
	if len(chunks) == 0 {
		return
	}

	// Collect unique file IDs and batch-fetch file metadata.
	fileIDSet := make(map[string]struct{})
	for _, chunk := range chunks {
		fileID, err := models.RecordIDString(chunk.File)
		if err != nil {
			logger.Warn("embed worker: failed to extract file ID from chunk", "error", err)
			continue
		}
		fileIDSet[fileID] = struct{}{}
	}
	fileIDs := make([]string, 0, len(fileIDSet))
	for id := range fileIDSet {
		fileIDs = append(fileIDs, id)
	}

	files, err := w.db.GetFilesByIDs(ctx, fileIDs)
	if err != nil {
		logger.Error("embed worker: get files by ids", "error", err)
		return
	}

	// Build file lookup: fileID → file.
	type fileInfo struct {
		title   string
		path    string
		vaultID string
	}
	fileLookup := make(map[string]fileInfo, len(files))
	for _, f := range files {
		fid, err := models.RecordIDString(f.ID)
		if err != nil {
			logger.Warn("embed worker: failed to extract file ID", "error", err)
			continue
		}
		vid, err := models.RecordIDString(f.Vault)
		if err != nil {
			logger.Warn("embed worker: failed to extract vault ID", "file_id", fid, "error", err)
			continue
		}
		fileLookup[fid] = fileInfo{title: f.Title, path: f.Path, vaultID: vid}
	}

	// Partition chunks into text and multimodal, filtering out no_embed files.
	var textPending []embeddingTask
	var mmPending []multimodalEmbeddingTask

	for _, chunk := range chunks {
		chunkID, err := models.RecordIDString(chunk.ID)
		if err != nil {
			logger.Warn("embed worker: failed to extract chunk ID", "error", err)
			continue
		}
		fileID, err := models.RecordIDString(chunk.File)
		if err != nil {
			logger.Warn("embed worker: failed to extract file ID from chunk", "chunk_id", chunkID, "error", err)
			continue
		}

		fi, ok := fileLookup[fileID]
		if !ok {
			// File was deleted after chunk was created — skip.
			continue
		}

		if !w.svc.shouldEmbed(ctx, fi.vaultID, fi.path) {
			// TODO: these chunks will be re-fetched every tick since they remain
			// with embedding IS NONE. Consider adding a DB-level filter or a
			// skip_embed flag on chunks to avoid repeated fetches.
			logger.Debug("embed worker: skipping no_embed file", "file_id", fileID, "path", fi.path)
			continue
		}

		if chunk.IsMultimodal() {
			if chunk.Hash == nil {
				logger.Warn("embed worker: multimodal chunk has no hash, skipping", "chunk_id", chunkID)
				continue
			}
			mmPending = append(mmPending, multimodalEmbeddingTask{
				chunkID:  chunkID,
				dataHash: *chunk.Hash,
				mimeType: chunk.MimeType,
				text:     buildEmbeddingContext(chunk, fi.title, w.svc.embedMaxInputChars),
			})
		} else {
			textPending = append(textPending, embeddingTask{
				chunkID: chunkID,
				text:    buildEmbeddingContext(chunk, fi.title, w.svc.embedMaxInputChars),
			})
		}
	}

	var total int

	if len(textPending) > 0 && embedder != nil {
		start := time.Now()
		n, err := w.svc.embedTextChunks(ctx, embedder, textPending)
		if err != nil {
			logger.Error("embed worker: text embedding failed", "error", err)
		}
		total += n
		if w.metrics != nil && n > 0 {
			w.metrics.RecordPipelineJob("embed", "completed", time.Since(start))
		}
	}

	if len(mmPending) > 0 {
		start := time.Now()
		n, err := w.svc.embedMultimodalChunks(ctx, mmEmbedder, embedder, mmPending)
		if err != nil {
			logger.Error("embed worker: multimodal embedding failed", "error", err)
		}
		total += n
		if w.metrics != nil && n > 0 {
			w.metrics.RecordPipelineJob("embed", "completed", time.Since(start))
		}
	}

	if total > 0 {
		logger.Info("embed worker: embedded chunks", "count", total, "text", len(textPending), "multimodal", len(mmPending))
	}
}
