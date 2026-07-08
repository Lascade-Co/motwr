# motwr

Standalone Go CLI that renders a branded 1080×1920 route short from a
TravelAnimator base video + a job JSON: edge-tts voiceover, karaoke captions,
title block, logo, bird overlays, vehicle ambience, speed-ramped to the
narration, brand outro appended.

## Requirements

- Go ≥ 1.26 (build only)
- `ffmpeg` / `ffprobe` on PATH (with libx264, libvpx, libass)
- Network access at render time (edge-tts)
- The committed `assets/` directory (brand media: bird webms, ambience
  tracks, logo, outro, fonts) — validated at startup, override with `-assets`

## Usage

    go build ./cmd/motwr
    ./motwr -job job.json -video base.mp4 -o out.mp4 [-assets ./assets] [-seed 42]

`-job` accepts a local path or an http(s) URL. The base video must be 9:16.

Job JSON:

    {"title": "Kochi to Goa", "subtitle": "1,200 km by road",
     "script": "Our journey begins...", "vehicle": "car"}

`vehicle` ∈ car | boat | plane | train.

## Docs

- `CONTEXT.md` — domain language
- `docs/superpowers/specs/` — design spec
- `docs/adr/` — decision records (why no whisper, why pure ffmpeg)

## Tests

    go test ./...                                   # unit
    MOTWR_TTS_LIVE=1 go test ./internal/tts/ -run Live   # live TTS
    MOTWR_E2E=1 go test ./cmd/motwr/ -timeout 10m        # full render
