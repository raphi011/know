package file

import (
	"context"
	"fmt"

	"github.com/raphi011/know/internal/event"
	"github.com/raphi011/know/internal/logutil"
	"github.com/raphi011/know/internal/models"
	"github.com/raphi011/know/internal/parser"
	"github.com/raphi011/know/internal/pipeline"
)

// ParseHandler returns a pipeline.Handler that runs the full text-file processing
// pipeline for a file: chunking, wiki-links, dangling link resolution, relations,
// tasks, external links, and label sync. On success it enqueues a "chunk" job so
// the embedding worker can pick up the freshly created chunks.
func ParseHandler(svc *Service, bus *event.Bus) pipeline.Handler {
	return func(ctx context.Context, job models.PipelineJob) error {
		fileID, err := models.RecordIDString(job.File)
		if err != nil {
			return fmt.Errorf("extract file id: %w", err)
		}

		doc, err := svc.db.GetFileByID(ctx, fileID)
		if err != nil {
			return fmt.Errorf("get file: %w", err)
		}
		if doc == nil {
			// File deleted — nothing to do.
			return nil
		}

		vaultID, err := models.RecordIDString(doc.Vault)
		if err != nil {
			return fmt.Errorf("extract vault id: %w", err)
		}

		// Template documents skip heavy processing — only sync labels.
		isTpl, err := svc.isTemplatePath(ctx, vaultID, doc.Path)
		if err != nil {
			return fmt.Errorf("check template path: %w", err)
		}
		if isTpl {
			if err := svc.db.SyncFileLabels(ctx, fileID, vaultID, doc.Labels); err != nil {
				return fmt.Errorf("sync labels for template %s: %w", doc.Path, err)
			}
			svc.publishFileEvent("file.processed", vaultID, doc)
			return nil
		}

		parsed := parser.ParseMarkdown(doc.Content)

		if err := svc.syncChunks(ctx, fileID, parsed, doc.Labels); err != nil {
			return fmt.Errorf("sync chunks for %s: %w", doc.Path, err)
		}
		if err := svc.processWikiLinks(ctx, fileID, vaultID, parsed.WikiLinks); err != nil {
			return fmt.Errorf("process wiki-links for %s: %w", doc.Path, err)
		}
		if err := svc.resolveDanglingForFile(ctx, vaultID, doc); err != nil {
			return fmt.Errorf("resolve dangling links for %s: %w", doc.Path, err)
		}
		if err := svc.processRelatesTo(ctx, fileID, vaultID, parsed.Frontmatter); err != nil {
			return fmt.Errorf("process relates_to for %s: %w", doc.Path, err)
		}
		if err := svc.syncTasks(ctx, fileID, vaultID, parsed.Tasks); err != nil {
			return fmt.Errorf("sync tasks for %s: %w", doc.Path, err)
		}
		if err := svc.processExternalLinks(ctx, fileID, vaultID, parsed.ExternalLinks); err != nil {
			return fmt.Errorf("process external links for %s: %w", doc.Path, err)
		}
		if err := svc.db.SyncFileLabels(ctx, fileID, vaultID, doc.Labels); err != nil {
			return fmt.Errorf("sync labels for %s: %w", doc.Path, err)
		}

		// Enqueue embed job so new chunks get embeddings.
		if svc.getEmbedder() != nil {
			if err := svc.db.CreateJob(ctx, fileID, "embed", 0); err != nil {
				return fmt.Errorf("create embed job for %s: %w", doc.Path, err)
			}
			if bus != nil {
				bus.Publish(event.ChangeEvent{Type: "job.created"})
			}
		}

		svc.publishFileEvent("file.processed", vaultID, doc)
		return nil
	}
}

// TranscribeHandler returns a pipeline.Handler that transcribes an audio file
// and stores the resulting chunks. On success it enqueues an "embed" job if an
// embedder is configured.
func TranscribeHandler(svc *Service, bus *event.Bus) pipeline.Handler {
	return func(ctx context.Context, job models.PipelineJob) error {
		fileID, err := models.RecordIDString(job.File)
		if err != nil {
			return fmt.Errorf("extract file id: %w", err)
		}

		doc, err := svc.db.GetFileByID(ctx, fileID)
		if err != nil {
			return fmt.Errorf("get file: %w", err)
		}
		if doc == nil {
			return nil
		}

		transcriber := svc.getTranscriber()
		if transcriber == nil {
			// Transcriber no longer configured — skip silently.
			return nil
		}

		if err := svc.transcribeFile(ctx, *transcriber, doc, fileID); err != nil {
			return fmt.Errorf("transcribe file: %w", err)
		}

		if svc.getEmbedder() != nil {
			if err := svc.db.CreateJob(ctx, fileID, "embed", 0); err != nil {
				return fmt.Errorf("create embed job after transcription: %w", err)
			}
			if bus != nil {
				bus.Publish(event.ChangeEvent{Type: "job.created"})
			}
		}

		vaultID, err := models.RecordIDString(doc.Vault)
		if err != nil {
			logutil.FromCtx(ctx).Warn("failed to extract vault id after transcription", "file_id", fileID, "error", err)
		} else {
			svc.publishFileEvent("file.processed", vaultID, doc)
		}
		return nil
	}
}

// EmbedHandler returns a pipeline.Handler that embeds all un-embedded chunks
// belonging to the job's file. This is the terminal step — no further job is created.
func EmbedHandler(svc *Service) pipeline.Handler {
	return func(ctx context.Context, job models.PipelineJob) error {
		fileID, err := models.RecordIDString(job.File)
		if err != nil {
			return fmt.Errorf("extract file id: %w", err)
		}

		n, err := svc.EmbedPendingChunksForFile(ctx, fileID)
		if err != nil {
			return fmt.Errorf("embed chunks: %w", err)
		}
		if n > 0 {
			logutil.FromCtx(ctx).Info("embed handler: embedded chunks", "file_id", fileID, "count", n)
		}
		return nil
	}
}
