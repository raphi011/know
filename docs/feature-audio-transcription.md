# Audio Transcription & Playback

Audio files can be automatically transcribed and made searchable. Transcription uses an async worker pipeline: audio files are ingested with their binary data, transcribed via a speech-to-text provider, and the resulting text is stored on the file record and chunked for embedding and search.

## How It Works

### Ingestion Flow

1. **Upload audio file** -- Audio files (`.mp3`, `.wav`, `.m4a`, `.ogg`, `.flac`, `.webm`, `.aac`, `.opus`) are ingested via `know cp` or the REST API, stored as binary files with their MIME type and content hash.
2. **ProcessingWorker schedules transcription** -- When an audio file is processed, the worker sets `transcribe_at = now` on the file record if an STT provider is configured. No chunks are created yet.
3. **TranscriptionWorker picks it up** -- An async worker polls for files with `transcribe_at <= now`, sends the audio to the configured STT provider, and receives timestamped transcript segments.
4. **Chunks created from transcript** -- Segments are grouped into time-window chunks (default 60s). Each chunk has `Text` (transcript text), `SourceLoc` (time range like `"1:00-2:00"`), and `EmbedAt` set for embedding. No audio binary is stored in chunks.
5. **File updated** -- The full stitched transcript is written to `file.Content`. The file now has both `Data` (original audio binary) and `Content` (full transcript text).
6. **Searchable** -- Chunks flow through the existing EmbeddingWorker for vector embedding. The transcript is searchable via both BM25 full-text and semantic vector search.

### Search Result Traceability

Search hits on transcript text point directly to the audio file with a time range in `SourceLoc`. No intermediate documents or extra hops -- the chunk's `File` reference is the audio file itself.

## Speech-to-Text Providers

### Provider Comparison

| Provider | Best For | Cost | Notes |
|---|---|---|---|
| **OpenAI Whisper API** | Cheapest cloud option | ~$0.006/min | Best accuracy in benchmarks; `gpt-4o-transcribe` model. 25MB file limit (auto-split via ffmpeg). |
| **AssemblyAI** | Feature-rich transcription | ~$0.006/min | Speaker diarization, PII redaction. Future provider candidate. |
| **Google Cloud STT** | Enterprise / GCP users | ~$0.006/min | Dedicated STT API, speaker diarization. Future provider candidate. |

### Current Implementation

**OpenAI Whisper API** (`gpt-4o-transcribe`) is the first supported provider. The `Transcriber` interface allows adding new providers behind `KNOW_STT_PROVIDER`.

## Architecture

### Pipeline Flow

```
Ingest audio file (file.Data = audio binary, processed=false)
  --> ProcessingWorker detects audio file
    --> Sets file.transcribe_at = now (no chunks created yet)
    --> Marks processed = true
  --> Publishes file.processed event

TranscriptionWorker wakes on file.processed
  --> Claims files where transcribe_at <= now
  --> For each file:
    1. If file.Data > 25MB: split via ffmpeg into <25MB parts
    2. Send audio to Whisper API (response_format=verbose_json)
    3. Get back segments with start/end timestamps + text
    4. Group segments into time-window chunks (default 60s)
    5. Create chunks: Text=transcript, SourceLoc="1:00-2:00", EmbedAt=now
    6. Stitch full transcript --> update file.Content
    7. Clear file.transcribe_at
  --> On failure: reschedule with 30s backoff

EmbeddingWorker (existing, unchanged)
  --> Picks up new chunks with embed_at set
  --> Embeds transcript text as usual
```

### Implemented Components

| Component | File | Description |
|-----------|------|-------------|
| Transcriber interface | `internal/stt/transcriber.go` | `Transcriber` interface, `Result`, `Segment` types |
| OpenAI provider | `internal/stt/openai.go` | Whisper API with verbose_json + segment timestamps |
| ffmpeg splitter | `internal/stt/ffmpeg.go` | Splits >25MB files for Whisper API limit |
| Provider factory | `internal/stt/factory.go` | Creates Transcriber from config |
| Segment grouping | `internal/pipeline/audio.go` | `GroupSegments` groups STT segments into time-window chunks |
| TranscriptionWorker | `internal/file/transcription_worker.go` | Async worker (same pattern as EmbeddingWorker) |
| Service methods | `internal/file/service.go` | `TranscribePendingFiles`, `transcribeFile`, audio branch in `ProcessFile` |
| File model | `internal/models/file.go` | `TranscribeAt` scheduling field |
| DB schema | `internal/db/schema.go` | `transcribe_at` column on file table |
| DB queries | `internal/db/queries_file.go` | Claim/update/reschedule/schedule transcription queries |
| Config | `internal/config/config.go` | STT provider/model/worker settings |
| Bootstrap | `internal/server/bootstrap.go` | Wires transcriber + worker lifecycle |

### Key Design Decisions

- **No audio binary in chunks** -- Avoids duplicating file data across N chunks. Audio stays on `File.Data` only. Chunks store only transcript text.
- **Whisper timestamps** -- `verbose_json` format returns segments with `start`/`end` times. Segments are grouped into configurable time windows (default 60s) for chunking.
- **ffmpeg only for >25MB files** -- Most audio files fit under Whisper's 25MB limit. ffmpeg splits larger files using stream copy (no re-encoding).
- **File-level transcription** -- The TranscriptionWorker operates on files (not chunks), keeping the flow simple: claim file → transcribe → create chunks → done.

## TUI Audio Player

The TUI includes an inline audio player with waveform visualization, allowing playback of audio files directly from the terminal. (Not yet implemented.)

### Libraries

- **[gopxl/beep v2](https://github.com/gopxl/beep)** -- Audio decoding (MP3, WAV, FLAC, OGG Vorbis) and playback via the `Streamer` interface.
- **Reference implementation**: [llehouerou/waves](https://github.com/llehouerou/waves) -- A Bubbletea audio player built with beep v2.

### UX Mockup

```
+-- standup-2026-03-14.mp3 ----------------------+
|  ▃▅▇█▆▃▁▂▅▇█▇▅▃▁▁▂▃▅▆▇█▇▅▃▂▁▃▅▇█▆▃▁▂▅▇▆▃▁▂▃▅▆  |
|       ^ 1:23 / 4:56                                |
|  > Playing    Vol: ████░░ 70%                       |
|  [space] play/pause  [<-/->] seek  [up/dn] volume  |
+-------------------------------------------------+
```

## Configuration

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `KNOW_STT_PROVIDER` | `none` | STT provider (`openai`, `none`) |
| `KNOW_STT_MODEL` | `gpt-4o-transcribe` | Model for transcription |
| `KNOW_TRANSCRIPTION_WORKER_INTERVAL` | `5` | Worker poll interval (seconds) |
| `KNOW_TRANSCRIPTION_WORKER_BATCH` | `5` | Files per worker tick |
| `KNOW_AUDIO_SEGMENT_SECONDS` | `60` | Max duration per transcript chunk (seconds) |

### Docker

The Docker image includes `ffmpeg` for splitting large audio files (>25MB). No additional setup needed.

## Example Prompts

- `know cp ~/recordings/ /meetings/ --vault default` -- Ingest a folder of audio recordings; transcripts are generated automatically when `KNOW_STT_PROVIDER=openai`.
- Search for meeting content: use the `search` MCP tool to find information mentioned in transcribed meetings.
- "What did we discuss in last week's standup?" -- Semantic search over transcribed meeting audio.
- "Find all meetings where we talked about the deployment pipeline" -- Full-text and vector search across transcripts.
