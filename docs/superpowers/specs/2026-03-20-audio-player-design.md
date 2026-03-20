# Audio Player in `know browse`

## Problem

Audio files in `know browse` render as raw text. Users must leave the TUI to play recordings. There is no way to listen to audio or see its waveform inline.

## Solution

Add a streaming audio player to the browse viewer. When a user opens a WAV audio file, they see a pre-computed waveform with playback controls above the transcript (if available). Audio streams from the server — playback starts before the full download completes.

## Design decisions

- **WAV-only playback** — other formats show transcript only. Avoids needing ffmpeg or Go decoding libraries at runtime.
- **Streaming download** — fetch WAV header (44 bytes) for duration/format, then download PCM body in a background goroutine. Playback starts immediately from the buffer. Handles remote servers with large files (~50MB for 5 min WAV).
- **malgo for playback** — same library used for recording. `malgo.Playback` mode with a data callback that reads from the growing PCM buffer.
- **Pre-computed waveform** — split PCM into N chunks (one per column, max 60), compute peak amplitude per chunk. Downloaded portions render in color, pending portions in dim `░`.
- **Auto-gain waveform** — same approach as recording: track peak amplitude seen so far, normalize all bars against it.

## Layout

```
─ /recordings/standup-2026-03-20.wav ───────────

  ▃▅▇█▆▃▁▂▅▇█▇▅▃▁░░░░░░░░░░░░░░░░░░░░░░░░░
       ▲ 0:12 / 4:56
  ▶ Playing (buffering...)
  space play/pause  ←/→ seek  esc back

  ── Transcript ─────────────────────────────
  We discussed the deployment pipeline and
  agreed to move the release to Thursday...
```

After download completes:

```
  ▃▅▇█▆▃▁▂▅▇█▇▅▃▁▁▂▃▅▆▇█▇▅▃▂▁▃▅▇█▆▃▁▂▅▇▆▃▁
                 ▲ 1:23 / 4:56
  ▶ Playing
  space play/pause  ←/→ seek  esc back
```

## Controls

| Key | Action |
|-----|--------|
| `space` | Play / pause |
| `←` | Seek back 5 seconds |
| `→` | Seek forward 5 seconds |
| `esc` | Stop playback, return to finder |

## Player state machine

```
loading → playing ⇄ paused → (esc) back to finder
               ↕
           seeking (←/→)
```

If playback position catches up to download cursor, auto-pause with "buffering..." indicator. Resume when enough data is available.

## Data flow

```
1. User selects audio file in finder
2. Viewer detects audio MIME type (from FileEntry.MimeType)
3. Viewer fetches document content (for transcript) + asset binary (for audio)
4. Player reads 44-byte WAV header → sample rate, channels, total PCM size, duration
5. Background goroutine downloads PCM body, appending to growing buffer
6. malgo Playback device starts → callback reads from buffer at playback offset
7. Tick (100ms) polls download progress → recomputes waveform for downloaded portion
8. Waveform: colored bars for downloaded data, dim ░ for pending
9. On esc: stop malgo device, clean up, return to finder
```

## Key types

```go
// Player manages streaming download + malgo playback.
type Player struct {
    mu          sync.Mutex
    pcmBuf      []byte        // grows as data downloads
    totalBytes  int           // expected total from WAV header
    downloaded  bool          // download complete?
    playOffset  int           // current playback byte position
    playing     bool          // actively playing?
    sampleRate  uint32
    channels    uint32
    bitsPerSample uint32
    duration    time.Duration

    ctx         *malgo.AllocatedContext
    device      *malgo.Device
}
```

## Files to create

| File | Purpose |
|------|---------|
| `internal/record/player.go` | Streaming download, malgo playback, seek/pause |
| `internal/record/wav.go` | Add `ParseWAVHeader()` to extract format + data size |
| `internal/record/amplitude.go` | Add `ComputeWaveform(pcm, width)` for pre-computed bars |
| `internal/tui/browse/audioplayer.go` | Bubbletea sub-model: waveform + controls + transcript viewport |

## Files to modify

| File | Change |
|------|--------|
| `internal/tui/browse/viewer.go` | Detect audio MIME → embed audioplayer instead of text viewport |
| `internal/apiclient/client.go` | Add `GetAssetReader(vaultID, hash)` returning streaming `io.ReadCloser` |
| `internal/api/ls.go` | Add `MimeType` and `ContentHash` to `FileEntry` API response |
| `internal/models/file.go` | Ensure `FileEntry` struct has `MimeType` and `ContentHash` fields |
| `internal/record/styles.go` | Add `bufferingBarStyle` for dim `░` characters |
| `docs/feature-audio-transcription.md` | Document `know browse` playback |

## Reusable code from recording feature

| What | Where |
|------|-------|
| `RenderWaveform()` | `internal/record/styles.go` — same block char rendering |
| `peakAmplitude()` | `internal/record/amplitude.go` — same amplitude calculation |
| Color palette | `internal/record/styles.go` — `primaryColor`, `mutedColor` |
| malgo context setup | `internal/record/recorder.go` — same `InitContext` pattern |
| WAV header format | `internal/record/wav.go` — same 44-byte structure, parse instead of write |

## Error handling

| Scenario | Behavior |
|----------|----------|
| Non-WAV audio file | Show transcript only with message: "Playback available for WAV files only" |
| No audio device | Show transcript only with message: "No audio output device found" |
| Download fails mid-stream | Pause playback, show error, user can still read transcript |
| No transcript available | Show player only, no transcript section |
| Corrupt WAV header | Show transcript only with error message |

## Testing

- `ParseWAVHeader()` — round-trip with `WriteWAV()`: write a WAV, parse it back, verify fields match
- `ComputeWaveform()` — known PCM input, verify bar count and amplitude values
- Player download/buffer logic — unit testable without malgo (mock the io.Reader)
- Manual: `know browse` → select WAV file → verify waveform + playback + transcript
