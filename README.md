# motwr

Standalone Go CLI that renders a branded 1080×1920 route short from a
TravelAnimator base video + a job JSON: edge-tts voiceover, karaoke captions,
title block, logo, bird overlays, vehicle ambience, speed-ramped to the
narration, brand outro appended.

## Getting Started

You don't need to know Go or programming to use motwr — just follow the
three steps below. You'll need an internet connection while rendering (the
voiceover is generated online).

### 1. Install ffmpeg (one-time setup)

motwr uses a free tool called **ffmpeg** to do the video work.

- **Mac**: open the Terminal app and paste:

      /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"
      brew install ffmpeg

  (The first line installs Homebrew, a Mac app installer. Skip it if you
  already have `brew`.)

- **Windows**: open PowerShell and paste:

      winget install ffmpeg

  Then close and reopen PowerShell so it's picked up.

- **Linux (Ubuntu/Debian)**:

      sudo apt install ffmpeg

To check it worked, type `ffmpeg -version` — you should see version info,
not an error.

### 2. Download motwr

Click the download link for your computer (always the newest version):

| Your computer | Download |
|---|---|
| Mac with Apple Silicon (M1/M2/M3/M4) | [motwr-darwin-arm64.zip](https://github.com/Lascade-Co/motwr/releases/latest/download/motwr-darwin-arm64.zip) |
| Mac with Intel | [motwr-darwin-amd64.zip](https://github.com/Lascade-Co/motwr/releases/latest/download/motwr-darwin-amd64.zip) |
| Windows | [motwr-windows-amd64.zip](https://github.com/Lascade-Co/motwr/releases/latest/download/motwr-windows-amd64.zip) |
| Linux | [motwr-linux-amd64.zip](https://github.com/Lascade-Co/motwr/releases/latest/download/motwr-linux-amd64.zip) |

(All versions are also on the [Releases page](https://github.com/Lascade-Co/motwr/releases).)

Unzip it anywhere (e.g. your Desktop). The folder already contains the
`motwr` program and an `assets` folder with all the branding media — keep
them together.

> **Mac only:** the first run may be blocked ("cannot be opened because the
> developer cannot be verified"). Fix it once with: right-click the `motwr`
> file → Open, or run `xattr -d com.apple.quarantine ./motwr` in Terminal
> from inside the folder.

### 3. Render a video

You need two things:

1. **Your base video** — the TravelAnimator map export (portrait 9:16).
   Copy it into the unzipped folder, e.g. as `route.mp4`.
2. **Your job link** — the `https://…` link you were given for this video.
   It points to the video's details (title, subtitle, narration script, and
   vehicle) and is used as-is; there's nothing to download or edit.

Then open Terminal (Mac/Linux) or PowerShell (Windows) **in that folder**
(Mac: right-click the folder → "New Terminal at Folder"; Windows:
shift-right-click → "Open PowerShell window here") and run — replacing the
link with your own:

- Mac/Linux:

      ./motwr -job "https://example.com/jobs/1234.json" -video route.mp4 -o finished.mp4

- Windows:

      .\motwr.exe -job "https://example.com/jobs/1234.json" -video route.mp4 -o finished.mp4

Keep the quotes around the link.

Progress lines will scroll by (`==> generating voiceover`,
`==> rendering main segment`…). After a minute or two, `finished.mp4`
appears in the same folder — that's your video, ready to upload.

If something goes wrong, the last line printed says what to fix — the most
common issues are a base video that isn't 9:16 portrait, a job link that
was pasted incompletely or has expired, or ffmpeg not installed (step 1).

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
