# Audio Transcription & Playback

Audio files can be automatically transcribed and made searchable. Transcription uses an async worker pipeline: audio files are ingested with their binary data, transcribed via a speech-to-text provider, and the resulting text is stored on the file record and chunked for embedding and search.

## How It Works

### Ingestion Flow

1. **Upload audio file** -- Audio files (`.mp3`, `.wav`, `.m4a`, `.ogg`, `.flac`, `.webm`, `.aac`, `.opus`) are ingested via `know import`, `know note record`, or the REST API, stored as binary files with their MIME type and content hash.
2. **Parse job** -- The pipeline worker creates a `transcribe` job for audio files when an STT provider is configured.
3. **Transcribe job** -- The worker sends the audio to the configured STT provider and receives timestamped transcript segments. Segments are grouped into time-window chunks (default 60s). Each chunk has `Text` (transcript text), `SourceLoc` (time range like `"1:00-2:00"`), and `EmbedAt` set for embedding. The full stitched transcript is written to `file.Content`.
4. **Summarize job** (optional) -- If an LLM is available and the vault has `transcript_template` configured, the raw transcript is summarized using the template via an LLM. The summary replaces `file.Content` and a re-parse job regenerates chunks from the summarized content.
5. **Embed job** -- Chunks flow through the embedding handler for vector embedding. The transcript is searchable via both BM25 full-text and semantic vector search.

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

### Implemented Components

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

### Key Design Decisions

- **No audio binary in chunks** -- Avoids duplicating file data across N chunks. Audio stays on `File.Data` only. Chunks store only transcript text.
- **Whisper timestamps** -- `verbose_json` format returns segments with `start`/`end` times. Segments are grouped into configurable time windows (default 60s) for chunking.
- **ffmpeg only for >25MB files** -- Most audio files fit under Whisper's 25MB limit. ffmpeg splits larger files using stream copy (no re-encoding).
- **File-level transcription** -- The transcribe handler operates on files (not chunks), keeping the flow simple: transcribe → create chunks → done.
- **Optional LLM summarization** -- When `transcript_template` is configured on the vault, the raw transcript is processed through an LLM with the template. The summary replaces the raw transcript and chunks are regenerated.

## TUI Audio Player

The TUI includes an inline audio player with waveform visualization, allowing playback of WAV files directly from the terminal via `know browse`.

### Libraries

- **[gen2brain/malgo](https://github.com/gen2brain/malgo)** -- Cross-platform audio I/O via miniaudio C bindings. Used for both recording and playback.

### Controls

| Key | Action |
|-----|--------|
| `space` | Play / pause |
| `←` / `→` | Seek -/+ 5 seconds |
| `+` / `-` | Volume up / down |
| `esc` | Back to file browser |

## Configuration

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `KNOW_STT_PROVIDER` | `none` | STT provider (`openai`, `none`) |
| `KNOW_STT_MODEL` | `gpt-4o-transcribe` | Model for transcription |
| `KNOW_STT_BASE_URL` | (empty) | Base URL for STT API (empty = OpenAI API; set for local whisper.cpp server) |
| `KNOW_AUDIO_SEGMENT_SECONDS` | `60` | Max duration per transcript chunk (seconds) |

### Docker

The Docker image includes `ffmpeg` for splitting large audio files (>25MB). No additional setup needed.

## CLI Audio Recording

Record audio directly from the terminal and upload to a vault for automatic transcription.

### Usage

```bash
# Record audio and upload to default vault
know note record

# Record to a specific vault and folder
know note record --vault meetings --path /standups/
```

### Controls

| Key | Action |
|-----|--------|
| `Enter` | Stop recording and upload |
| `Escape` | Cancel and discard |

### TUI

The recording interface shows a real-time waveform visualization using Unicode block characters, elapsed time, and hotkey hints:

```
● Recording  1:23
▁▂▃▅▇█▆▃▂▁▃▅▇█▇▅▃▂▁▁▂▄▆▇█▇▅▃▁▂▃▅▇
enter save  esc cancel
```

Recordings are saved as WAV files (44100 Hz, 16-bit, mono) and uploaded to the vault at `/recordings/recording-YYYY-MM-DD-HHMMSS.wav`. The server's transcription pipeline processes them automatically.

### Dependencies

Audio capture uses [malgo](https://github.com/gen2brain/malgo) (miniaudio Go bindings), which requires CGO. On macOS, no additional system libraries are needed (uses CoreAudio). On Linux, requires `-ldl` (libc dynamic loader).

## Playback in Browse

WAV audio files can be played back directly in `know browse`. When you open a WAV file, the viewer shows a waveform visualization with playback controls above the transcript (if available).

### Usage

```bash
# Browse vault and select an audio file
know browse --vault default

# Or open a specific audio file directly
know browse /recordings/standup-2026-03-20.wav --vault default
```

### Controls

| Key | Action |
|-----|--------|
| `space` | Play / pause |
| `←` | Seek back 5 seconds |
| `→` | Seek forward 5 seconds |
| `esc` | Stop playback, return to finder |
| `q` | Quit |

### Limitations

- **WAV only** — playback is currently supported for `.wav` files only. Other audio formats (MP3, OGG, etc.) show the transcript without playback controls.
- Requires an audio output device on the machine running `know browse`.

## Example Prompts

- `know note record` -- Record a voice note; transcript is generated automatically when `KNOW_STT_PROVIDER=openai`.
- `know import ~/recordings/ /meetings/ --vault default` -- Ingest a folder of audio recordings; transcripts are generated automatically when `KNOW_STT_PROVIDER=openai`.
- Search for meeting content: use the `search` MCP tool to find information mentioned in transcribed meetings.
- "What did we discuss in last week's standup?" -- Semantic search over transcribed meeting audio.
- "Find all meetings where we talked about the deployment pipeline" -- Full-text and vector search across transcripts.
