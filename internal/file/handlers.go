package file

import (
	"context"
	"fmt"
	"time"

	"github.com/raphi011/know/internal/event"
	"github.com/raphi011/know/internal/logutil"
	"github.com/raphi011/know/internal/models"
	"github.com/raphi011/know/internal/parser"
	"github.com/raphi011/know/internal/pipeline"
)

// ParseHandler returns a pipeline.Handler that runs the full text-file processing
// pipeline for a file: chunking, wiki-links, dangling link resolution, relations,
// tasks, external links, and label sync. On success it enqueues an "embed" job so
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

		// Enqueue summarize job if LLM is available — the handler checks
		// whether the vault has a transcript_template configured.
		if svc.getModel() != nil {
			if err := svc.db.CreateJob(ctx, fileID, "summarize", 0); err != nil {
				return fmt.Errorf("create summarize job: %w", err)
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

// SummarizeHandler returns a pipeline.Handler that uses an LLM to summarize
// an audio transcript using a vault template. If no transcript_template is
// configured on the vault, the handler completes without action.
func SummarizeHandler(svc *Service, bus *event.Bus) pipeline.Handler {
	return func(ctx context.Context, job models.PipelineJob) error {
		logger := logutil.FromCtx(ctx)

		fileID, err := models.RecordIDString(job.File)
		if err != nil {
			return fmt.Errorf("extract file id: %w", err)
		}

		model := svc.getModel()
		if model == nil {
			logger.Warn("LLM model not available, skipping summarization", "file_id", fileID)
			return nil
		}

		doc, err := svc.db.GetFileByID(ctx, fileID)
		if err != nil {
			return fmt.Errorf("get file: %w", err)
		}
		if doc == nil {
			logger.Warn("file deleted before summarization", "file_id", fileID)
			return nil
		}

		vaultID, err := models.RecordIDString(doc.Vault)
		if err != nil {
			return fmt.Errorf("extract vault id: %w", err)
		}

		// Check vault setting for transcript template
		vault, err := svc.db.GetVault(ctx, vaultID)
		if err != nil {
			return fmt.Errorf("get vault: %w", err)
		}
		if vault == nil {
			logger.Warn("vault not found, skipping summarization", "vault_id", vaultID, "file_id", fileID)
			return nil
		}
		templatePath := vault.Defaults().TranscriptTemplate
		if templatePath == "" {
			// No template configured — nothing to do.
			return nil
		}

		// Fetch template content
		tpl, err := svc.db.GetFileByPath(ctx, vaultID, templatePath)
		if err != nil {
			return fmt.Errorf("get template %s: %w", templatePath, err)
		}
		if tpl == nil {
			logger.Warn("transcript template not found, skipping summarization",
				"template_path", templatePath, "vault", vaultID)
			return nil
		}

		if doc.Content == "" {
			logger.Warn("file has no transcript content, skipping summarization", "file_id", fileID)
			return nil
		}

		// Apply template variables before LLM fill
		templateContent := ApplyTemplateVars(tpl.Content, DefaultTemplateVars(time.Now(), doc.Path, vault.Name))

		logger.Info("summarizing transcript with template",
			"file_id", fileID, "template", templatePath, "transcript_len", len(doc.Content))

		summary, err := model.FillTemplate(ctx, templateContent, doc.Content)
		if err != nil {
			return fmt.Errorf("fill template: %w", err)
		}

		// Overwrite the raw transcript with the LLM-rendered summary.
		if err := svc.db.UpdateFileTranscript(ctx, fileID, summary); err != nil {
			return fmt.Errorf("update summary: %w", err)
		}

		// Re-parse so chunks are regenerated from the summary content.
		if err := svc.db.CreateJob(ctx, fileID, "parse", 0); err != nil {
			return fmt.Errorf("create parse job after summarization: %w", err)
		}
		if bus != nil {
			bus.Publish(event.ChangeEvent{Type: "job.created"})
		}

		doc.Content = summary
		svc.publishFileEvent("file.processed", vaultID, doc)
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
