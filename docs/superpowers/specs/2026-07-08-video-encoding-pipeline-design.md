# motwr — Video Encoding Pipeline CLI (Design)

Date: 2026-07-08
Status: Approved

## Purpose

A standalone Go CLI that turns a TravelAnimator-style base video plus a job
payload (title, subtitle, narration script, vehicle) into a finished
1080×1920 short: TTS voiceover, karaoke captions, title block, logo, bird
overlays with sound effects, vehicle ambience, speed-ramped to the narration,
with the brand outro appended.

Replaces the Node/Remotion render server (`server-setup` reference) with a
single binary whose only external dependency is `ffmpeg`/`ffprobe` on PATH.

## Inputs

```
motwr -job <path-or-url> -video <base.mp4> -o <out.mp4> [-assets <dir>] [-seed <n>]
```

- `-job` — job JSON, either a local file path or an `http(s)://` URL.
- `-video` — local mp4, the base (map animation) video. **Must be 9:16
  aspect ratio**; validated with ffprobe before any work starts, error
  otherwise. A same-ratio, non-1080×1920 input is scaled to 1080×1920.
- `-o` — output mp4 path.
- `-assets` — assets directory (default `./assets`).
- `-seed` — RNG seed for reproducible bird selection (default: time-based).

### Job JSON schema (minimal)

```json
{
  "title": "Kochi to Goa",
  "subtitle": "1,200 km by road",
  "script": "Our journey begins in Kochi...",
  "vehicle": "car"
}
```

- `vehicle` ∈ {car, boat, plane, train}; anything else errors.
- All four fields required and non-empty; unknown fields ignored.

### Assets directory layout (existing)

```
assets/
├── bird/0.webm … 3.webm     # VP9 alpha, 1920×1080, 3–13 s each
├── background/{car,boat,plane,train}.mp3
├── bird-sfx.mp3             # 28.7 s, shared by all bird appearances
├── logo.png                 # 444×222
├── outro.mp4                # 1080×1920@30, h264 + aac 44.1 kHz, ~3 s
└── fonts/Anton-Regular.ttf, Montserrat-Bold.ttf
```

Asset presence is validated up front; missing files are a startup error.

## Pipeline stages

All orchestration in Go; media work shells out to ffmpeg/ffprobe. Temp files
live in one per-run temp dir, removed on exit (including on error).

1. **Load job** — read local file or GET the URL; parse + validate JSON.
2. **Validate** — ffprobe the base video (must be 9:16); check all assets.
3. **TTS** — native Go implementation of the edge-tts websocket protocol
   (voice `en-US-GuyNeural`). One connection yields the MP3 voiceover and
   `WordBoundary` events (per-word offset + duration, 100 ns ticks) → word
   timestamps in memory. No whisper, no python.
4. **Probe durations** — voiceover duration `A`, base video duration `V`;
   speed factor `A/V` applied as `setpts` (slows down or speeds up so the
   base video spans exactly the narration).
5. **Caption pages** — group word timestamps into pages of ≤ 1.2 s (TikTok
   style, as in the reference). Generate one ASS subtitle file containing:
   - **Title block** — top-center, visible for the whole main segment.
     Title: Anton 88 px, white, uppercase, letter-spaced, drop shadow.
     Subtitle: Montserrat Bold 32 px, gold `#FFD700`, uppercase.
   - **Karaoke captions** — dark pill (`rgba(0,0,0,0.6)`, rounded),
     Montserrat 42 px centered ~35% from top; one Dialogue event per word
     interval rendering the page's full text with only the active word in
     `#FFD700`, the rest white.
   Fonts are loaded from `assets/fonts/` via the subtitles filter's
   `fontsdir` option.
6. **Bird schedule** — gap-based: the first appearance starts at t=0 and
   each subsequent one starts 10 s after the previous clip ends, so
   appearances never overlap; each appearance picks a random clip (seeded
   RNG) and plays its natural length. Clips are decoded with `-c:v libvpx-vp9` (required to keep the
   alpha channel), scale-cropped 1920×1080 → 1080×1920 cover. Each
   appearance also schedules a bird-sfx instance: `adelay` to the
   appearance start, trimmed to the clip length, fade-out, volume 0.4.
   The sfx deliberately stays a separate file rather than being muxed
   into the webms: it is 28.7 s against 3–13 s clips, the filter graph
   would need to extract/delay/mix the audio stream either way, and a
   separate file keeps volume and trim decisions in the pipeline instead
   of baked into slow VP9-alpha re-encodes.
7. **Main render (single ffmpeg pass)** — one filter graph:
   - video: base `setpts` ramp + scale to 1080×1920 → bird overlays
     (`overlay` + `enable=between(t,…)`) → logo top-right (40 px padding,
     fit within ~218 px box) → `subtitles=captions.ass:fontsdir=…`
   - audio: `amix` of voiceover ×1.5, `background/{vehicle}.mp3` ×0.3
     (trimmed to main duration, fade-out), bird-sfx instances ×0.4
   - encode: h264 CRF 18 `yuv420p`, 30 fps, aac 44.1 kHz stereo, duration
     exactly the voiceover length — parameters chosen to match outro.mp4.
8. **Concat outro** — concat demuxer with `-c copy` (codecs match by
   construction); if copy fails, fall back to a re-encoding concat filter.

## Error handling

- Fail fast, before encoding, on: unreadable/invalid job JSON, bad vehicle,
  non-9:16 input, missing assets, missing ffmpeg/ffprobe.
- Every ffmpeg/ffprobe invocation captures stderr; on failure the CLI exits
  non-zero printing the command and its stderr tail.
- TTS websocket failures are retried a few times, then fatal.

## Package structure

```
cmd/motwr/main.go      # flag parsing, wiring
internal/job/          # JSON load (file/URL) + validation
internal/tts/          # edge-tts websocket client → audio + word stamps
internal/captions/     # word → pages → ASS generation
internal/schedule/     # bird appearance schedule (seeded)
internal/ffmpeg/       # probe, filter-graph builder, run wrapper, concat
```

Each package is independently testable; the filter-graph builder and ASS
generator are pure functions from inputs to strings.

## Testing

- Unit tests: caption paging, ASS output (golden files), bird scheduling,
  filter-graph construction, job JSON validation, 9:16 check logic.
- Integration test (build-tagged / skipped without ffmpeg): render a short
  sample end-to-end and assert output duration, dimensions, stream layout.

## Out of scope

- Location images, caption style variants 2–5, total distance overlay
  (reference features not in this spec).
- ElevenLabs support (edge-tts only, for now).
- Server/queue mode — this is a one-shot CLI.
