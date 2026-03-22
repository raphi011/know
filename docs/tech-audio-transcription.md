# Audio Transcription — Architecture

> For user-facing documentation (usage, configuration, example prompts), see [feature-audio-transcription.md](feature-audio-transcription.md).

## Pipeline Flow

```
Ingest audio file (file.Data = audio binary)
  --> PipelineWorker creates "transcribe" job
  --> transcribe handler:
    1. If file.Data > 25MB: split via ffmpeg into <25MB parts
    2. Send audio to STT provider (response_format=verbose_json)
    3. Get back segments with start/end timestamps + text
    4. Group segments into time-window chunks (default 60s)
    5. Create chunks: Text=transcript, SourceLoc="1:00-2:00"
    6. Stitch full transcript --> update file.Content
    7. Enqueue "embed" job + "summarize" job (if LLM available)
  --> summarize handler (optional):
    1. Check vault.transcript_template setting
    2. If set: render template with LLM, overwrite file.Content
    3. Enqueue "parse" job to regenerate chunks from summary
  --> embed handler:
    Embeds transcript/summary chunks as usual
```

## Implemented Components

| Component | File | Description |
|-----------|------|-------------|
| Transcriber interface | `internal/stt/transcriber.go` | `Transcriber` interface, `Result`, `Segment` types |
| OpenAI provider | `internal/stt/openai.go` | Whisper API with verbose_json + segment timestamps |
| ffmpeg splitter | `internal/stt/ffmpeg.go` | Splits >25MB files for Whisper API limit |
| Provider factory | `internal/stt/factory.go` | Creates Transcriber from config |
| Pipeline handlers | `internal/file/handlers.go` | `TranscribeHandler`, `SummarizeHandler` |
| Service methods | `internal/file/service.go` | `transcribeFile`, `SetTranscriber`, `SetModel` |
| File model | `internal/models/file.go` | Audio file fields |
| Vault settings | `internal/models/vault.go` | `TranscriptTemplate` setting |
| Config | `internal/config/config.go` | STT provider/model/base URL settings |
| Bootstrap | `internal/server/bootstrap.go` | Wires transcriber + LLM + pipeline worker |

## Key Design Decisions

- **No audio binary in chunks** -- Avoids duplicating file data across N chunks. Audio stays on `File.Data` only. Chunks store only transcript text.
- **Whisper timestamps** -- `verbose_json` format returns segments with `start`/`end` times. Segments are grouped into configurable time windows (default 60s) for chunking.
- **ffmpeg only for >25MB files** -- Most audio files fit under Whisper's 25MB limit. ffmpeg splits larger files using stream copy (no re-encoding).
- **File-level transcription** -- The transcribe handler operates on files (not chunks), keeping the flow simple: transcribe → create chunks → done.
- **Optional LLM summarization** -- When `transcript_template` is configured on the vault, the raw transcript is processed through an LLM with the template. The summary replaces the raw transcript and chunks are regenerated.
