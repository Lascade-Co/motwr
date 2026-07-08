# motwr Video Encoding Pipeline Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A standalone Go CLI (`motwr`) that renders a 1080×1920 branded short from a base video + job JSON: edge-tts voiceover with word timestamps, karaoke captions + title block + logo + bird overlays via one ffmpeg pass, vehicle ambience, uniform speed ramp, outro appended by stream-copy concat.

**Architecture:** Go orchestrates; all media work shells out to `ffmpeg`/`ffprobe`. TTS is a native Go implementation of the edge-tts websocket protocol (audio + WordBoundary events in one connection — no whisper, see ADR-0001). All overlays are burned in a single ffmpeg filter graph with captions/title as generated ASS via libass (see ADR-0002).

**Tech Stack:** Go ≥1.22, `github.com/coder/websocket`, `golang.org/x/image` (font metrics), system `ffmpeg`/`ffprobe` (7.x, with libx264, libvpx, libass).

## Global Constraints

- Module path: `github.com/lascade/motwr`; binary `motwr`.
- Only two Go dependencies: `github.com/coder/websocket`, `golang.org/x/image`.
- Output: 1080×1920, 30 fps CFR, h264 CRF 18 yuv420p, aac 192k 44100 Hz stereo — chosen to stream-copy-concat with `assets/outro.mp4`.
- Base video MUST be 9:16 (±0.005 ratio tolerance) or the CLI errors before any work.
- TTS voice hardcoded: `en-US-GuyNeural`. No flag (explicit decision).
- Speed Ramp = uniform retime, single `setpts` factor = voiceoverDur/videoDur.
- Bird schedule: gap-based — first Appearance at t=0, next starts 10 s after previous ends; clips play natural length; skip appearance if <0.5 s remains.
- Audio mix: voiceover ×1.5, background ambience ×0.3 (looped, 1.5 s fade-out), bird-sfx ×0.4 per appearance (trimmed to clip, 0.3 s fade-out).
- Title: Anton starting 88 px, shrink-to-fit 90% of width (972 px), floor 48 px else "title too long" error. Subtitle: Montserrat Bold 32 px gold `#FFD700`.
- Caption Pages: ≤1.2 s per page; active word gold `#FFD700`, others white; dark pill = ASS BorderStyle=3, BackColour `&H66000000` (rounded corners are a known, accepted deviation from the reference).
- Vehicle enum: {car, boat, plane, train} — anything else is a job validation error.
- All temp files in one per-run temp dir, removed on exit including on error.
- Every ffmpeg/ffprobe failure surfaces the command + stderr tail.
- Tests that need ffmpeg call `requireFFmpeg(t)` (skip if absent); live-network TTS test gated by `MOTWR_TTS_LIVE=1`; full E2E gated by `MOTWR_E2E=1`.

---

### Task 1: Module scaffold + job package

**Files:**
- Create: `go.mod`, `.gitignore`
- Create: `internal/job/job.go`
- Test: `internal/job/job_test.go`

**Interfaces:**
- Consumes: nothing (first task)
- Produces:
  - `type Vehicle string` with constants `VehicleCar/VehicleBoat/VehiclePlane/VehicleTrain`
  - `type Job struct { Title, Subtitle, Script string; Vehicle Vehicle }` (json tags `title`, `subtitle`, `script`, `vehicle`)
  - `func Load(pathOrURL string) (*Job, error)` — local file or http(s) GET (30 s timeout)
  - `func Parse(data []byte) (*Job, error)` — unmarshal + validate

- [ ] **Step 1: Init module and .gitignore**

```bash
cd /Users/rohittp/Data/Lascade/motwr
go mod init github.com/lascade/motwr
printf 'motwr\n.DS_Store\n.idea/\n.venv/\n' > .gitignore
```

- [ ] **Step 2: Write the failing tests**

`internal/job/job_test.go`:

```go
package job

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const validJSON = `{"title":"Kochi to Goa","subtitle":"1,200 km by road","script":"Our journey begins.","vehicle":"car"}`

func TestParseValid(t *testing.T) {
	j, err := Parse([]byte(validJSON))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if j.Title != "Kochi to Goa" || j.Subtitle != "1,200 km by road" ||
		j.Script != "Our journey begins." || j.Vehicle != VehicleCar {
		t.Fatalf("unexpected job: %+v", j)
	}
}

func TestParseUnknownFieldsIgnored(t *testing.T) {
	if _, err := Parse([]byte(`{"title":"a","subtitle":"b","script":"c","vehicle":"boat","id":42}`)); err != nil {
		t.Fatalf("unknown fields must be ignored: %v", err)
	}
}

func TestParseRejectsBadInput(t *testing.T) {
	cases := map[string]string{
		"bad vehicle":    `{"title":"a","subtitle":"b","script":"c","vehicle":"rocket"}`,
		"empty title":    `{"title":"","subtitle":"b","script":"c","vehicle":"car"}`,
		"empty subtitle": `{"title":"a","subtitle":"","script":"c","vehicle":"car"}`,
		"empty script":   `{"title":"a","subtitle":"b","script":"","vehicle":"car"}`,
		"missing fields": `{"title":"a"}`,
		"not json":       `nope`,
	}
	for name, in := range cases {
		if _, err := Parse([]byte(in)); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}

func TestLoadFromFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "job.json")
	if err := os.WriteFile(p, []byte(validJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	j, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if j.Vehicle != VehicleCar {
		t.Fatalf("got %+v", j)
	}
}

func TestLoadFromURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(validJSON))
	}))
	defer srv.Close()
	j, err := Load(srv.URL + "/job.json")
	if err != nil {
		t.Fatalf("Load URL: %v", err)
	}
	if j.Title != "Kochi to Goa" {
		t.Fatalf("got %+v", j)
	}
}

func TestLoadURLNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "gone", http.StatusNotFound)
	}))
	defer srv.Close()
	if _, err := Load(srv.URL); err == nil || !strings.Contains(err.Error(), "404") {
		t.Fatalf("expected 404 error, got %v", err)
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./internal/job/`
Expected: FAIL (package does not compile — `Parse` undefined)

- [ ] **Step 4: Write the implementation**

`internal/job/job.go`:

```go
// Package job loads and validates the render Job payload.
package job

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

type Vehicle string

const (
	VehicleCar   Vehicle = "car"
	VehicleBoat  Vehicle = "boat"
	VehiclePlane Vehicle = "plane"
	VehicleTrain Vehicle = "train"
)

// Job is the minimal render payload (see spec: minimal job JSON schema).
type Job struct {
	Title    string  `json:"title"`
	Subtitle string  `json:"subtitle"`
	Script   string  `json:"script"`
	Vehicle  Vehicle `json:"vehicle"`
}

func (j *Job) validate() error {
	switch {
	case strings.TrimSpace(j.Title) == "":
		return fmt.Errorf("job: title is required")
	case strings.TrimSpace(j.Subtitle) == "":
		return fmt.Errorf("job: subtitle is required")
	case strings.TrimSpace(j.Script) == "":
		return fmt.Errorf("job: script is required")
	}
	switch j.Vehicle {
	case VehicleCar, VehicleBoat, VehiclePlane, VehicleTrain:
		return nil
	default:
		return fmt.Errorf("job: vehicle must be one of car|boat|plane|train, got %q", j.Vehicle)
	}
}

// Parse unmarshals and validates a job payload.
func Parse(data []byte) (*Job, error) {
	var j Job
	if err := json.Unmarshal(data, &j); err != nil {
		return nil, fmt.Errorf("job: invalid JSON: %w", err)
	}
	if err := j.validate(); err != nil {
		return nil, err
	}
	return &j, nil
}

// Load reads the job from a local path or an http(s) URL.
func Load(pathOrURL string) (*Job, error) {
	var data []byte
	if strings.HasPrefix(pathOrURL, "http://") || strings.HasPrefix(pathOrURL, "https://") {
		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Get(pathOrURL)
		if err != nil {
			return nil, fmt.Errorf("job: fetch %s: %w", pathOrURL, err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("job: fetch %s: HTTP %d", pathOrURL, resp.StatusCode)
		}
		data, err = io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		if err != nil {
			return nil, fmt.Errorf("job: read %s: %w", pathOrURL, err)
		}
	} else {
		var err error
		data, err = os.ReadFile(pathOrURL)
		if err != nil {
			return nil, fmt.Errorf("job: %w", err)
		}
	}
	return Parse(data)
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/job/`
Expected: `ok github.com/lascade/motwr/internal/job`

- [ ] **Step 6: Commit**

```bash
git add go.mod .gitignore internal/job/
git commit -m "feat: job payload loading and validation"
```

---

### Task 2: ffmpeg probe wrapper + 9:16 validation + run wrapper

**Files:**
- Create: `internal/ffmpeg/probe.go`
- Create: `internal/ffmpeg/run.go`
- Test: `internal/ffmpeg/probe_test.go`

**Interfaces:**
- Consumes: nothing
- Produces:
  - `type ProbeResult struct { Width, Height int; Duration float64 }`
  - `func Probe(ctx context.Context, path string) (*ProbeResult, error)` (Width/Height zero for audio-only files)
  - `func CheckAspect916(w, h int) error`
  - `func Run(ctx context.Context, args []string) error` — runs `ffmpeg` with `-hide_banner -v error` prepended, captures stderr, error includes command + stderr tail
  - `func requireFFmpeg(t *testing.T)` test helper (skips when ffmpeg/ffprobe absent) — test file only

- [ ] **Step 1: Write the failing tests**

`internal/ffmpeg/probe_test.go`:

```go
package ffmpeg

import (
	"context"
	"math"
	"os/exec"
	"path/filepath"
	"testing"
)

func requireFFmpeg(t *testing.T) {
	t.Helper()
	for _, bin := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s not on PATH", bin)
		}
	}
}

// makeFixture renders a tiny test video: 540x960 (9:16), 2s, 30fps, with a sine audio track.
func makeFixture(t *testing.T) string {
	t.Helper()
	out := filepath.Join(t.TempDir(), "fixture.mp4")
	cmd := exec.Command("ffmpeg", "-hide_banner", "-v", "error", "-y",
		"-f", "lavfi", "-i", "testsrc=duration=2:size=540x960:rate=30",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=2",
		"-c:v", "libx264", "-pix_fmt", "yuv420p", "-c:a", "aac", "-shortest", out)
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("fixture: %v\n%s", err, b)
	}
	return out
}

func TestProbeFixture(t *testing.T) {
	requireFFmpeg(t)
	r, err := Probe(context.Background(), makeFixture(t))
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if r.Width != 540 || r.Height != 960 {
		t.Errorf("got %dx%d, want 540x960", r.Width, r.Height)
	}
	if math.Abs(r.Duration-2.0) > 0.2 {
		t.Errorf("duration %.3f, want ~2.0", r.Duration)
	}
}

func TestProbeMissingFile(t *testing.T) {
	requireFFmpeg(t)
	if _, err := Probe(context.Background(), "/nonexistent/file.mp4"); err == nil {
		t.Fatal("expected error")
	}
}

func TestCheckAspect916(t *testing.T) {
	for _, tc := range []struct {
		w, h int
		ok   bool
	}{
		{1080, 1920, true},
		{540, 960, true},
		{720, 1280, true},
		{1920, 1080, false},
		{1080, 1080, false},
		{0, 1920, false},
	} {
		err := CheckAspect916(tc.w, tc.h)
		if tc.ok && err != nil {
			t.Errorf("%dx%d: unexpected error %v", tc.w, tc.h, err)
		}
		if !tc.ok && err == nil {
			t.Errorf("%dx%d: expected error", tc.w, tc.h)
		}
	}
}

func TestRunFailureIncludesStderr(t *testing.T) {
	requireFFmpeg(t)
	err := Run(context.Background(), []string{"-i", "/nonexistent/file.mp4", "-f", "null", "-"})
	if err == nil {
		t.Fatal("expected error")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/ffmpeg/`
Expected: FAIL — `Probe` undefined

- [ ] **Step 3: Write the implementation**

`internal/ffmpeg/probe.go`:

```go
// Package ffmpeg wraps ffprobe/ffmpeg invocations and builds the render
// filter graph.
package ffmpeg

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os/exec"
	"strconv"
)

type ProbeResult struct {
	Width, Height int
	Duration      float64
}

type probeJSON struct {
	Streams []struct {
		CodecType string `json:"codec_type"`
		Width     int    `json:"width"`
		Height    int    `json:"height"`
	} `json:"streams"`
	Format struct {
		Duration string `json:"duration"`
	} `json:"format"`
}

// Probe returns container duration and, when a video stream exists, its
// dimensions.
func Probe(ctx context.Context, path string) (*ProbeResult, error) {
	cmd := exec.CommandContext(ctx, "ffprobe", "-v", "error",
		"-print_format", "json", "-show_format", "-show_streams", path)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ffprobe %s: %w (%s)", path, err, exitStderr(err))
	}
	var pj probeJSON
	if err := json.Unmarshal(out, &pj); err != nil {
		return nil, fmt.Errorf("ffprobe %s: bad JSON: %w", path, err)
	}
	r := &ProbeResult{}
	r.Duration, _ = strconv.ParseFloat(pj.Format.Duration, 64)
	for _, s := range pj.Streams {
		if s.CodecType == "video" {
			r.Width, r.Height = s.Width, s.Height
			break
		}
	}
	if r.Duration <= 0 {
		return nil, fmt.Errorf("ffprobe %s: no duration", path)
	}
	return r, nil
}

// CheckAspect916 enforces the 9:16 input contract (spec: aspect ratio is
// validated, not corrected).
func CheckAspect916(w, h int) error {
	if w <= 0 || h <= 0 {
		return fmt.Errorf("base video has no valid dimensions (%dx%d)", w, h)
	}
	if math.Abs(float64(w)/float64(h)-9.0/16.0) > 0.005 {
		return fmt.Errorf("base video must be 9:16 portrait, got %dx%d", w, h)
	}
	return nil
}
```

`internal/ffmpeg/run.go`:

```go
package ffmpeg

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// Run executes ffmpeg with the given args; on failure the error carries the
// full command and the tail of stderr.
func Run(ctx context.Context, args []string) error {
	full := append([]string{"-hide_banner", "-v", "error"}, args...)
	cmd := exec.CommandContext(ctx, "ffmpeg", full...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ffmpeg %s: %w\n%s",
			strings.Join(full, " "), err, tail(stderr.String(), 2000))
	}
	return nil
}

func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return "…" + s[len(s)-n:]
}

func exitStderr(err error) string {
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return tail(string(ee.Stderr), 500)
	}
	return ""
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/ffmpeg/`
Expected: PASS (all 4 tests, none skipped on this machine)

- [ ] **Step 5: Commit**

```bash
git add internal/ffmpeg/
git commit -m "feat: ffprobe wrapper, 9:16 validation, ffmpeg run wrapper"
```

---

### Task 3: Fonts + title text measurement (shrink-to-fit)

**Files:**
- Create: `assets/fonts/Anton-Regular.ttf`, `assets/fonts/Montserrat-Bold.ttf` (downloaded, OFL-licensed), `assets/fonts/OFL-Anton.txt`, `assets/fonts/OFL-Montserrat.txt`
- Create: `internal/captions/measure.go`
- Test: `internal/captions/measure_test.go`

**Interfaces:**
- Consumes: nothing
- Produces:
  - `func LoadFont(path string) (*sfnt.Font, error)`
  - `func TextWidth(f *sfnt.Font, text string, sizePx, letterSpacing float64) (float64, error)`
  - `func FitTitleSize(f *sfnt.Font, text string, maxWidth, startSize, floorSize, letterSpacing float64) (float64, error)` — returns the size to use; error mentioning "title too long" when the floor is hit. Callers pass the already-uppercased title.

- [ ] **Step 1: Download and commit the fonts**

```bash
mkdir -p assets/fonts
curl -fsSL -o assets/fonts/Anton-Regular.ttf \
  https://raw.githubusercontent.com/google/fonts/main/ofl/anton/Anton-Regular.ttf
curl -fsSL -o assets/fonts/OFL-Anton.txt \
  https://raw.githubusercontent.com/google/fonts/main/ofl/anton/OFL.txt
curl -fsSL -o assets/fonts/Montserrat-Bold.ttf \
  https://raw.githubusercontent.com/JulietaUla/Montserrat/master/fonts/ttf/Montserrat-Bold.ttf
curl -fsSL -o assets/fonts/OFL-Montserrat.txt \
  https://raw.githubusercontent.com/JulietaUla/Montserrat/master/OFL.txt
# sanity: both parse as TTF
file assets/fonts/*.ttf
```

Expected: `file` reports TrueType font data for both. If the Montserrat URL 404s, the repo default branch may have changed — check `https://github.com/JulietaUla/Montserrat` for the `fonts/ttf/` path.

- [ ] **Step 2: Write the failing tests**

`internal/captions/measure_test.go`:

```go
package captions

import (
	"strings"
	"testing"
)

const antonPath = "../../assets/fonts/Anton-Regular.ttf"

func TestTextWidthPositiveAndMonotonic(t *testing.T) {
	f, err := LoadFont(antonPath)
	if err != nil {
		t.Fatalf("LoadFont: %v", err)
	}
	w88, err := TextWidth(f, "KOCHI TO GOA", 88, 2)
	if err != nil {
		t.Fatal(err)
	}
	w44, err := TextWidth(f, "KOCHI TO GOA", 44, 2)
	if err != nil {
		t.Fatal(err)
	}
	if w88 <= 0 || w44 <= 0 || w88 <= w44 {
		t.Fatalf("widths not monotonic: w88=%.1f w44=%.1f", w88, w44)
	}
}

func TestFitTitleSizeShortTitleKeepsStartSize(t *testing.T) {
	f, _ := LoadFont(antonPath)
	size, err := FitTitleSize(f, "GOA", 972, 88, 48, 2)
	if err != nil || size != 88 {
		t.Fatalf("size=%v err=%v, want 88", size, err)
	}
}

func TestFitTitleSizeShrinksLongTitle(t *testing.T) {
	f, _ := LoadFont(antonPath)
	size, err := FitTitleSize(f, "SAN FRANCISCO TO NEW YORK CITY", 972, 88, 48, 2)
	if err != nil {
		t.Fatal(err)
	}
	if size >= 88 || size < 48 {
		t.Fatalf("size=%.1f, want in [48,88)", size)
	}
	w, _ := TextWidth(f, "SAN FRANCISCO TO NEW YORK CITY", size, 2)
	if w > 972 {
		t.Fatalf("shrunk text still too wide: %.1f", w)
	}
}

func TestFitTitleSizeFloorErrors(t *testing.T) {
	f, _ := LoadFont(antonPath)
	long := strings.Repeat("VERY LONG TITLE ", 8)
	if _, err := FitTitleSize(f, long, 972, 88, 48, 2); err == nil ||
		!strings.Contains(err.Error(), "title too long") {
		t.Fatalf("expected 'title too long' error, got %v", err)
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./internal/captions/`
Expected: FAIL — `LoadFont` undefined

- [ ] **Step 4: Write the implementation**

```bash
go get golang.org/x/image@latest
```

`internal/captions/measure.go`:

```go
// Package captions turns word timestamps into caption pages and generates
// the ASS subtitle file (title block + karaoke captions).
package captions

import (
	"fmt"
	"os"
	"unicode/utf8"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
)

func LoadFont(path string) (*sfnt.Font, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("font: %w", err)
	}
	f, err := opentype.Parse(b)
	if err != nil {
		return nil, fmt.Errorf("font %s: %w", path, err)
	}
	return f, nil
}

// TextWidth returns the advance width in pixels of text at sizePx, including
// letterSpacing px between characters (matching ASS \fsp behaviour).
func TextWidth(f *sfnt.Font, text string, sizePx, letterSpacing float64) (float64, error) {
	face, err := opentype.NewFace(f, &opentype.FaceOptions{
		Size: sizePx, DPI: 72, Hinting: font.HintingNone,
	})
	if err != nil {
		return 0, err
	}
	defer face.Close()
	w := float64(font.MeasureString(face, text)) / 64.0
	if n := utf8.RuneCountInString(text); n > 1 {
		w += letterSpacing * float64(n-1)
	}
	return w, nil
}

// FitTitleSize shrinks startSize until text fits maxWidth. Advance widths
// scale linearly with size, so one ratio step converges; letterSpacing stays
// constant, hence the small refinement loop.
func FitTitleSize(f *sfnt.Font, text string, maxWidth, startSize, floorSize, letterSpacing float64) (float64, error) {
	size := startSize
	for range 5 {
		w, err := TextWidth(f, text, size, letterSpacing)
		if err != nil {
			return 0, err
		}
		if w <= maxWidth {
			break
		}
		size *= maxWidth / w
	}
	if size < floorSize {
		return 0, fmt.Errorf("title too long: would need %.0fpx font (floor %.0fpx)", size, floorSize)
	}
	return size, nil
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/captions/`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add assets/fonts/ internal/captions/ go.mod go.sum
git commit -m "feat: bundle OFL fonts, title shrink-to-fit measurement"
```

---

### Task 4: Bird schedule

**Files:**
- Create: `internal/schedule/schedule.go`
- Test: `internal/schedule/schedule_test.go`

**Interfaces:**
- Consumes: nothing
- Produces:
  - `type Appearance struct { Clip int; Start, End float64 }` (seconds; Clip indexes the caller's clip-duration slice)
  - `func Build(mainDuration float64, clipDurations []float64, gap float64, rng *rand.Rand) []Appearance`

- [ ] **Step 1: Write the failing tests**

`internal/schedule/schedule_test.go`:

```go
package schedule

import (
	"math/rand"
	"testing"
)

var clips = []float64{3.033, 4.366, 4.366, 12.9}

func TestGapBasedNoOverlap(t *testing.T) {
	apps := Build(60, clips, 10, rand.New(rand.NewSource(1)))
	if len(apps) == 0 {
		t.Fatal("no appearances")
	}
	if apps[0].Start != 0 {
		t.Errorf("first appearance must start at t=0, got %.3f", apps[0].Start)
	}
	for i, a := range apps {
		if a.Clip < 0 || a.Clip >= len(clips) {
			t.Errorf("appearance %d: bad clip %d", i, a.Clip)
		}
		if a.End <= a.Start {
			t.Errorf("appearance %d: End %.3f <= Start %.3f", i, a.End, a.Start)
		}
		if i > 0 {
			wantStart := apps[i-1].End + 10
			if a.Start != wantStart {
				t.Errorf("appearance %d: Start %.3f, want prev End+10 = %.3f", i, a.Start, wantStart)
			}
		}
	}
}

func TestClipTrimmedAtMainEnd(t *testing.T) {
	apps := Build(60, clips, 10, rand.New(rand.NewSource(1)))
	last := apps[len(apps)-1]
	if last.End > 60 {
		t.Errorf("last appearance End %.3f exceeds main duration 60", last.End)
	}
}

func TestSkipsTinyTailAppearance(t *testing.T) {
	// Main duration leaves only 0.3s after the first clip + gap: no 2nd bird.
	first := clips[0] // seeded rng with 1 clip choice space
	apps := Build(first+10+0.3, []float64{first}, 10, rand.New(rand.NewSource(1)))
	if len(apps) != 1 {
		t.Fatalf("expected exactly 1 appearance, got %d", len(apps))
	}
}

func TestDeterministicForSeed(t *testing.T) {
	a := Build(120, clips, 10, rand.New(rand.NewSource(42)))
	b := Build(120, clips, 10, rand.New(rand.NewSource(42)))
	if len(a) != len(b) {
		t.Fatalf("lengths differ: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("appearance %d differs: %+v vs %+v", i, a[i], b[i])
		}
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/schedule/`
Expected: FAIL — `Build` undefined

- [ ] **Step 3: Write the implementation**

`internal/schedule/schedule.go`:

```go
// Package schedule computes gap-based Bird Appearance timings: first at t=0,
// each next starting `gap` seconds after the previous clip ends, clips
// playing their natural length (see CONTEXT.md "Bird Appearance").
package schedule

import "math/rand"

const minAppearance = 0.5 // seconds; shorter tails are skipped

type Appearance struct {
	Clip       int     // index into the clip-duration slice passed to Build
	Start, End float64 // seconds within the Main Segment
}

func Build(mainDuration float64, clipDurations []float64, gap float64, rng *rand.Rand) []Appearance {
	var out []Appearance
	t := 0.0
	for mainDuration-t >= minAppearance {
		clip := rng.Intn(len(clipDurations))
		end := t + clipDurations[clip]
		if end > mainDuration {
			end = mainDuration
		}
		out = append(out, Appearance{Clip: clip, Start: t, End: end})
		t = end + gap
	}
	return out
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/schedule/`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/schedule/
git commit -m "feat: gap-based bird appearance schedule"
```

---

### Task 5: Caption pages

**Files:**
- Create: `internal/captions/pages.go`
- Test: `internal/captions/pages_test.go`

**Interfaces:**
- Consumes: nothing new
- Produces:
  - `type Word struct { Text string; Start, End float64 }` (seconds)
  - `type Page struct { Words []Word }` with method `func (p Page) Start() float64`
  - `func BuildPages(words []Word, maxPageDur float64) []Page` — a word starts a new page when `word.Start - page.Start() >= maxPageDur`

- [ ] **Step 1: Write the failing tests**

`internal/captions/pages_test.go`:

```go
package captions

import "testing"

func w(text string, start, end float64) Word { return Word{Text: text, Start: start, End: end} }

func TestBuildPagesGroupsWithinWindow(t *testing.T) {
	words := []Word{
		w("Our", 0.0, 0.3), w("journey", 0.35, 0.8), w("begins", 0.85, 1.1),
		w("in", 1.3, 1.4), w("Kochi", 1.5, 2.0),
	}
	pages := BuildPages(words, 1.2)
	if len(pages) != 2 {
		t.Fatalf("got %d pages, want 2", len(pages))
	}
	if len(pages[0].Words) != 3 || len(pages[1].Words) != 2 {
		t.Fatalf("page sizes %d/%d, want 3/2", len(pages[0].Words), len(pages[1].Words))
	}
	if pages[1].Start() != 1.3 {
		t.Errorf("page 2 start %.2f, want 1.30", pages[1].Start())
	}
}

func TestBuildPagesSingleWordPages(t *testing.T) {
	words := []Word{w("one", 0, 0.5), w("two", 2, 2.5), w("three", 4, 4.5)}
	pages := BuildPages(words, 1.2)
	if len(pages) != 3 {
		t.Fatalf("got %d pages, want 3", len(pages))
	}
}

func TestBuildPagesEmpty(t *testing.T) {
	if pages := BuildPages(nil, 1.2); len(pages) != 0 {
		t.Fatalf("expected no pages, got %d", len(pages))
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/captions/`
Expected: FAIL — `Word` undefined

- [ ] **Step 3: Write the implementation**

`internal/captions/pages.go`:

```go
package captions

// Word is one TTS word timestamp in seconds.
type Word struct {
	Text       string
	Start, End float64
}

// Page is a group of consecutive words displayed together (CONTEXT.md
// "Caption Page").
type Page struct {
	Words []Word
}

func (p Page) Start() float64 { return p.Words[0].Start }

// BuildPages groups words TikTok-style: a word joins the current page while
// it starts within maxPageDur of the page start.
func BuildPages(words []Word, maxPageDur float64) []Page {
	var pages []Page
	for _, w := range words {
		if len(pages) == 0 || w.Start-pages[len(pages)-1].Start() >= maxPageDur {
			pages = append(pages, Page{})
		}
		p := &pages[len(pages)-1]
		p.Words = append(p.Words, w)
	}
	return pages
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/captions/`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/captions/pages.go internal/captions/pages_test.go
git commit -m "feat: caption page grouping"
```

---

### Task 6: ASS generation (title block + karaoke captions)

**Files:**
- Create: `internal/captions/ass.go`
- Test: `internal/captions/ass_test.go`

**Interfaces:**
- Consumes: `Page`, `Word` (Task 5)
- Produces:
  - `func GenerateASS(title, subtitle string, titleSize float64, pages []Page, mainDuration float64) string` — callers pass raw (not uppercased) title/subtitle; uppercasing happens inside. `titleSize` comes from `FitTitleSize` on the UPPERCASED title.
  - Exported layout constants used by main: `TitleStartSize = 88.0`, `TitleFloorSize = 48.0`, `TitleMaxWidth = 972.0`, `TitleLetterSpacing = 2.0`, `MaxPageDur = 1.2`

- [ ] **Step 1: Write the failing tests**

`internal/captions/ass_test.go`:

```go
package captions

import (
	"strings"
	"testing"
)

func testPages() []Page {
	return BuildPages([]Word{
		w("Our", 0.0, 0.3), w("journey", 0.35, 0.8), w("begins", 0.85, 1.1),
		w("in", 1.3, 1.4), w("Kochi", 1.5, 2.0),
	}, MaxPageDur)
}

func TestGenerateASSHeaderAndStyles(t *testing.T) {
	out := GenerateASS("Kochi to Goa", "1,200 km by road", 88, testPages(), 30)
	for _, want := range []string{
		"PlayResX: 1080", "PlayResY: 1920",
		"Style: Title,Anton,88,", "Style: Subtitle,Montserrat,32,",
		"Style: Caption,Montserrat,42,",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q", want)
		}
	}
}

func TestGenerateASSTitleBlock(t *testing.T) {
	out := GenerateASS("Kochi to Goa", "1,200 km by road", 72, testPages(), 30)
	if !strings.Contains(out, "KOCHI TO GOA") {
		t.Error("title not uppercased")
	}
	if !strings.Contains(out, "1,200 KM BY ROAD") {
		t.Error("subtitle not uppercased")
	}
	if !strings.Contains(out, "Style: Title,Anton,72,") {
		t.Error("computed title size not baked into style")
	}
	// Title visible for the whole main segment.
	if !strings.Contains(out, "Dialogue: 0,0:00:00.00,0:00:30.00,Title,") {
		t.Error("title event should span 0 to main duration")
	}
}

func TestGenerateASSKaraokeHighlight(t *testing.T) {
	out := GenerateASS("T", "S", 88, testPages(), 30)
	// Active word gold, inactive restored to white.
	if !strings.Contains(out, `{\1c&H00D7FF&}Our{\1c&HFFFFFF&}`) {
		t.Error("missing gold highlight for first word")
	}
	if !strings.Contains(out, `{\1c&H00D7FF&}journey{\1c&HFFFFFF&}`) {
		t.Error("missing gold highlight for second word")
	}
	// The event during the first word runs 0.00 -> 0.30.
	if !strings.Contains(out, "Dialogue: 0,0:00:00.00,0:00:00.30,Caption,") {
		t.Error("missing first word interval event")
	}
}

func TestGenerateASSGapShowsAllWhite(t *testing.T) {
	out := GenerateASS("T", "S", 88, testPages(), 30)
	// Between "Our"(ends 0.30) and "journey"(starts 0.35) no word is active:
	// there must be an event for 0.30->0.35 whose text has no gold override.
	idx := strings.Index(out, "Dialogue: 0,0:00:00.30,0:00:00.35,Caption,")
	if idx < 0 {
		t.Fatal("missing gap event")
	}
	line := out[idx:]
	line = line[:strings.Index(line, "\n")]
	if strings.Contains(line, `\1c&H00D7FF&`) {
		t.Error("gap event must not highlight any word")
	}
}

func TestGenerateASSSanitizesBraces(t *testing.T) {
	pages := BuildPages([]Word{w("hi{x}", 0, 0.5)}, MaxPageDur)
	out := GenerateASS("T{", "S}", 88, pages, 10)
	if strings.Contains(out, "{x}") || strings.Contains(out, "T{,") {
		t.Error("braces must be sanitized out of user text")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/captions/`
Expected: FAIL — `GenerateASS` undefined

- [ ] **Step 3: Write the implementation**

`internal/captions/ass.go`:

```go
package captions

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// Layout constants (PlayRes units == output pixels, 1080x1920).
const (
	TitleStartSize     = 88.0
	TitleFloorSize     = 48.0
	TitleMaxWidth      = 972.0 // 90% of 1080
	TitleLetterSpacing = 2.0
	MaxPageDur         = 1.2

	titleY      = 200.0
	subtitleGap = 18.0
	captionY    = 1250.0

	goldInline  = `{\1c&H00D7FF&}` // #FFD700 in ASS BBGGRR
	whiteInline = `{\1c&HFFFFFF&}`
)

// GenerateASS renders the full subtitle file: Title Block events plus one
// Caption event per word-highlight interval.
func GenerateASS(title, subtitle string, titleSize float64, pages []Page, mainDuration float64) string {
	var b strings.Builder
	b.WriteString("[Script Info]\n")
	b.WriteString("ScriptType: v4.00+\n")
	b.WriteString("PlayResX: 1080\nPlayResY: 1920\n")
	b.WriteString("WrapStyle: 2\nScaledBorderAndShadow: yes\n\n")

	b.WriteString("[V4+ Styles]\n")
	b.WriteString("Format: Name, Fontname, Fontsize, PrimaryColour, SecondaryColour, OutlineColour, BackColour, Bold, Italic, Underline, StrikeOut, ScaleX, ScaleY, Spacing, Angle, BorderStyle, Outline, Shadow, Alignment, MarginL, MarginR, MarginV, Encoding\n")
	// Title: Anton, white, slight shadow. Alignment 8 = top-center.
	fmt.Fprintf(&b, "Style: Title,Anton,%s,&H00FFFFFF,&H00FFFFFF,&H00000000,&HA0000000,0,0,0,0,100,100,%g,0,1,0,2,8,54,54,0,1\n",
		trimFloat(titleSize), TitleLetterSpacing)
	// Subtitle: Montserrat Bold, gold (#FFD700 -> &H0000D7FF).
	b.WriteString("Style: Subtitle,Montserrat,32,&H0000D7FF,&H0000D7FF,&H00000000,&HA0000000,-1,0,0,0,100,100,3,0,1,0,1,8,54,54,0,1\n")
	// Caption: Montserrat Bold on a semi-transparent black box (BorderStyle=3,
	// box colour = BackColour; rgba(0,0,0,0.6) -> alpha 0x66).
	b.WriteString("Style: Caption,Montserrat,42,&H00FFFFFF,&H00FFFFFF,&H66000000,&H66000000,-1,0,0,0,100,100,0,0,3,12,0,8,54,54,0,1\n\n")

	b.WriteString("[Events]\n")
	b.WriteString("Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\n")

	end := assTime(mainDuration)
	// Title Block: title line then subtitle line below it.
	fmt.Fprintf(&b, "Dialogue: 0,0:00:00.00,%s,Title,,0,0,0,,{\\an8\\pos(540,%s)}%s\n",
		end, trimFloat(titleY), sanitize(strings.ToUpper(title)))
	subY := titleY + titleSize*1.05 + subtitleGap
	fmt.Fprintf(&b, "Dialogue: 0,0:00:00.00,%s,Subtitle,,0,0,0,,{\\an8\\pos(540,%s)}%s\n",
		end, trimFloat(subY), sanitize(strings.ToUpper(subtitle)))

	// Karaoke caption events.
	for i, p := range pages {
		dispEnd := p.Words[len(p.Words)-1].End
		if next := i + 1; next < len(pages) && pages[next].Start() < dispEnd {
			dispEnd = pages[next].Start()
		}
		if dispEnd > mainDuration {
			dispEnd = mainDuration
		}
		for _, iv := range pageIntervals(p, dispEnd) {
			fmt.Fprintf(&b, "Dialogue: 0,%s,%s,Caption,,0,0,0,,{\\an5\\pos(540,%s)}%s\n",
				assTime(iv.start), assTime(iv.end), trimFloat(captionY), pageText(p, iv.active))
		}
	}
	return b.String()
}

type interval struct {
	start, end float64
	active     int // index into page words, -1 = none
}

// pageIntervals slices [pageStart, dispEnd] at every word start/end so each
// slice has a constant active word (or none, during inter-word gaps).
func pageIntervals(p Page, dispEnd float64) []interval {
	bounds := []float64{p.Start(), dispEnd}
	for _, w := range p.Words {
		for _, t := range []float64{w.Start, w.End} {
			if t > p.Start() && t < dispEnd {
				bounds = append(bounds, t)
			}
		}
	}
	sort.Float64s(bounds)
	var out []interval
	for i := 0; i+1 < len(bounds); i++ {
		s, e := bounds[i], bounds[i+1]
		if e-s < 0.001 {
			continue
		}
		active := -1
		for wi, w := range p.Words {
			if w.Start <= s+0.0005 && w.End > s+0.0005 {
				active = wi
				break
			}
		}
		out = append(out, interval{start: s, end: e, active: active})
	}
	return out
}

func pageText(p Page, active int) string {
	parts := make([]string, len(p.Words))
	for i, w := range p.Words {
		t := sanitize(w.Text)
		if i == active {
			t = goldInline + t + whiteInline
		}
		parts[i] = t
	}
	return strings.Join(parts, " ")
}

// sanitize strips ASS override syntax from user-supplied text.
func sanitize(s string) string {
	s = strings.ReplaceAll(s, "{", "(")
	s = strings.ReplaceAll(s, "}", ")")
	s = strings.ReplaceAll(s, "\n", " ")
	return s
}

func assTime(t float64) string {
	if t < 0 {
		t = 0
	}
	cs := int(math.Round(t * 100))
	h := cs / 360000
	cs %= 360000
	m := cs / 6000
	cs %= 6000
	return fmt.Sprintf("%d:%02d:%02d.%02d", h, m, cs/100, cs%100)
}

// trimFloat renders 88 not 88.000000, but keeps 72.5.
func trimFloat(f float64) string {
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.2f", f), "0"), ".")
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/captions/`
Expected: PASS (all measure, pages, and ass tests)

- [ ] **Step 5: Commit**

```bash
git add internal/captions/ass.go internal/captions/ass_test.go
git commit -m "feat: ASS generation for title block and karaoke captions"
```

---

### Task 7: edge-tts client (Go-native websocket)

**Files:**
- Create: `internal/tts/gec.go` (DRM token)
- Create: `internal/tts/protocol.go` (message parsing — pure)
- Create: `internal/tts/edge.go` (websocket client)
- Test: `internal/tts/gec_test.go`, `internal/tts/protocol_test.go`, `internal/tts/edge_live_test.go`

**Interfaces:**
- Consumes: nothing
- Produces:
  - `type WordStamp struct { Text string; Start, End time.Duration }`
  - `func Synthesize(ctx context.Context, script, outPath string) ([]WordStamp, error)` — writes MP3, returns stamps sorted by Start; retries 3× on transient failure; voice hardcoded `en-US-GuyNeural` (ADR-0001)

- [ ] **Step 1: Write the failing tests for the pure parts**

`internal/tts/gec_test.go`:

```go
package tts

import (
	"regexp"
	"testing"
	"time"
)

func TestGenerateSecMSGECFormat(t *testing.T) {
	tok := GenerateSecMSGEC(time.Date(2026, 7, 8, 12, 0, 0, 0, time.UTC))
	if !regexp.MustCompile(`^[0-9A-F]{64}$`).MatchString(tok) {
		t.Fatalf("token %q is not 64 uppercase hex chars", tok)
	}
}

func TestGenerateSecMSGECStableWithin5MinWindow(t *testing.T) {
	base := time.Date(2026, 7, 8, 12, 0, 1, 0, time.UTC)
	if GenerateSecMSGEC(base) != GenerateSecMSGEC(base.Add(4*time.Minute)) {
		t.Error("token must be stable inside one 5-minute window")
	}
	if GenerateSecMSGEC(base) == GenerateSecMSGEC(base.Add(6*time.Minute)) {
		t.Error("token must change across 5-minute windows")
	}
}
```

`internal/tts/protocol_test.go`:

```go
package tts

import (
	"testing"
	"time"
)

func TestSplitTextMessage(t *testing.T) {
	msg := "X-RequestId:abc\r\nPath:audio.metadata\r\n\r\n{\"Metadata\":[]}"
	h, body := splitTextMessage(msg)
	if h["Path"] != "audio.metadata" {
		t.Errorf("Path=%q", h["Path"])
	}
	if body != `{"Metadata":[]}` {
		t.Errorf("body=%q", body)
	}
}

func TestParseBinaryMessage(t *testing.T) {
	header := "Path:audio\r\nContent-Type:audio/mpeg"
	payload := []byte{0xFF, 0xF3, 0x01, 0x02}
	msg := append([]byte{0, byte(len(header))}, []byte(header)...)
	msg = append(msg, payload...)
	h, got, err := parseBinaryMessage(msg)
	if err != nil {
		t.Fatal(err)
	}
	if h["Path"] != "audio" {
		t.Errorf("Path=%q", h["Path"])
	}
	if string(got) != string(payload) {
		t.Errorf("payload=%v", got)
	}
}

func TestParseBinaryMessageTooShort(t *testing.T) {
	if _, _, err := parseBinaryMessage([]byte{0}); err == nil {
		t.Error("expected error")
	}
}

func TestParseMetadataWordBoundary(t *testing.T) {
	body := `{"Metadata":[
	  {"Type":"WordBoundary","Data":{"Offset":1000000,"Duration":4000000,"text":{"Text":"Our"}}},
	  {"Type":"SentenceBoundary","Data":{"Offset":0,"Duration":0,"text":{"Text":"x"}}},
	  {"Type":"WordBoundary","Data":{"Offset":5500000,"Duration":3000000,"text":{"Text":"journey"}}}]}`
	stamps, err := parseMetadata([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	if len(stamps) != 2 {
		t.Fatalf("got %d stamps, want 2 (SentenceBoundary skipped)", len(stamps))
	}
	if stamps[0].Text != "Our" || stamps[0].Start != 100*time.Millisecond ||
		stamps[0].End != 500*time.Millisecond {
		t.Errorf("stamp[0] = %+v", stamps[0])
	}
	if stamps[1].Text != "journey" || stamps[1].Start != 550*time.Millisecond {
		t.Errorf("stamp[1] = %+v", stamps[1])
	}
}

func TestSSMLEscapesScript(t *testing.T) {
	s := buildSSML("Fish & chips <fast>")
	if want := "Fish &amp; chips &lt;fast&gt;"; !strings.Contains(s, want) {
		t.Errorf("ssml %q missing %q", s, want)
	}
	if !strings.Contains(s, "en-US-GuyNeural") {
		t.Error("ssml missing hardcoded voice")
	}
}
```

(Add `"strings"` to the test file's imports.)

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/tts/`
Expected: FAIL — `GenerateSecMSGEC` undefined

- [ ] **Step 3: Implement the pure parts**

`internal/tts/gec.go`:

```go
// Package tts implements the edge-tts websocket protocol natively: one
// connection yields both the voiceover MP3 and per-word WordBoundary
// timestamps (ADR-0001).
package tts

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

const trustedClientToken = "6A5AA1D4EAFF4E9FB37E23D68491D6F4"

// GenerateSecMSGEC computes the Sec-MS-GEC DRM header: SHA-256 of the
// current Windows file time (100ns ticks since 1601-01-01) rounded down to a
// 5-minute boundary, concatenated with the trusted client token.
func GenerateSecMSGEC(now time.Time) string {
	const windowsEpochDiff = 11644473600 // seconds 1601-01-01 -> 1970-01-01
	sec := now.UTC().Unix() + windowsEpochDiff
	sec -= sec % 300
	ticks := sec * 10_000_000
	sum := sha256.Sum256([]byte(fmt.Sprintf("%d%s", ticks, trustedClientToken)))
	return strings.ToUpper(hex.EncodeToString(sum[:]))
}
```

`internal/tts/protocol.go`:

```go
package tts

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"html"
	"strings"
	"time"
)

const Voice = "en-US-GuyNeural"

type WordStamp struct {
	Text       string
	Start, End time.Duration
}

func splitTextMessage(msg string) (map[string]string, string) {
	headerPart, body, _ := strings.Cut(msg, "\r\n\r\n")
	headers := map[string]string{}
	for _, line := range strings.Split(headerPart, "\r\n") {
		if k, v, ok := strings.Cut(line, ":"); ok {
			headers[k] = v
		}
	}
	return headers, body
}

// parseBinaryMessage splits an audio frame: 2-byte big-endian header length,
// text headers, then raw payload.
func parseBinaryMessage(msg []byte) (map[string]string, []byte, error) {
	if len(msg) < 2 {
		return nil, nil, fmt.Errorf("tts: binary message too short (%d bytes)", len(msg))
	}
	hlen := int(binary.BigEndian.Uint16(msg[:2]))
	if len(msg) < 2+hlen {
		return nil, nil, fmt.Errorf("tts: header length %d exceeds message", hlen)
	}
	headers, _ := splitTextMessage(string(msg[2 : 2+hlen]))
	return headers, msg[2+hlen:], nil
}

type metadataPayload struct {
	Metadata []struct {
		Type string `json:"Type"`
		Data struct {
			Offset   int64 `json:"Offset"`   // 100ns ticks
			Duration int64 `json:"Duration"` // 100ns ticks
			Text     struct {
				Text string `json:"Text"`
			} `json:"text"`
		} `json:"Data"`
	} `json:"Metadata"`
}

func parseMetadata(body []byte) ([]WordStamp, error) {
	var mp metadataPayload
	if err := json.Unmarshal(body, &mp); err != nil {
		return nil, fmt.Errorf("tts: metadata: %w", err)
	}
	var out []WordStamp
	for _, m := range mp.Metadata {
		if m.Type != "WordBoundary" {
			continue
		}
		start := time.Duration(m.Data.Offset * 100)
		out = append(out, WordStamp{
			Text:  m.Data.Text.Text,
			Start: start,
			End:   start + time.Duration(m.Data.Duration*100),
		})
	}
	return out, nil
}

func buildSSML(script string) string {
	return fmt.Sprintf("<speak version='1.0' xmlns='http://www.w3.org/2001/10/synthesis' xml:lang='en-US'>"+
		"<voice name='%s'><prosody pitch='+0Hz' rate='+0%%' volume='+0%%'>%s</prosody></voice></speak>",
		Voice, html.EscapeString(script))
}
```

- [ ] **Step 4: Run pure tests to verify they pass**

Run: `go test ./internal/tts/`
Expected: PASS

- [ ] **Step 5: Implement the websocket client**

```bash
go get github.com/coder/websocket@latest
```

`internal/tts/edge.go`:

```go
package tts

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/coder/websocket"
)

const (
	wssBase    = "wss://speech.platform.bing.com/consumer/speech/synthesize/readaloud/edge/v1"
	gecVersion = "1-130.0.2849.68"
	userAgent  = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36 Edg/130.0.2849.68"
	origin     = "chrome-extension://jdiccldimpdaibmpdkjnbmckianbfold"
	// One websocket turn only; ample for narration scripts.
	maxScriptBytes = 32 << 10
)

// Synthesize generates the voiceover MP3 at outPath and returns word
// timestamps. Retries transient failures 3 times.
func Synthesize(ctx context.Context, script, outPath string) ([]WordStamp, error) {
	if len(script) > maxScriptBytes {
		return nil, fmt.Errorf("tts: script too long (%d bytes, max %d)", len(script), maxScriptBytes)
	}
	var lastErr error
	for attempt := range 3 {
		stamps, err := synthesizeOnce(ctx, script, outPath)
		if err == nil {
			return stamps, nil
		}
		lastErr = err
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Duration(attempt+1) * 2 * time.Second):
		}
	}
	return nil, fmt.Errorf("tts: all attempts failed: %w", lastErr)
}

func synthesizeOnce(ctx context.Context, script, outPath string) ([]WordStamp, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	url := fmt.Sprintf("%s?TrustedClientToken=%s&Sec-MS-GEC=%s&Sec-MS-GEC-Version=%s&ConnectionId=%s",
		wssBase, trustedClientToken, GenerateSecMSGEC(time.Now()), gecVersion, randomHex16())
	hdr := http.Header{}
	hdr.Set("Origin", origin)
	hdr.Set("User-Agent", userAgent)
	hdr.Set("Pragma", "no-cache")
	hdr.Set("Cache-Control", "no-cache")

	conn, _, err := websocket.Dial(ctx, url, &websocket.DialOptions{HTTPHeader: hdr})
	if err != nil {
		return nil, fmt.Errorf("tts: dial: %w", err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "")
	conn.SetReadLimit(1 << 22)

	ts := time.Now().UTC().Format("Mon Jan 02 2006 15:04:05 GMT+0000 (Coordinated Universal Time)")
	cfg := "X-Timestamp:" + ts + "\r\nContent-Type:application/json; charset=utf-8\r\nPath:speech.config\r\n\r\n" +
		`{"context":{"synthesis":{"audio":{"metadataoptions":{"sentenceBoundaryEnabled":"false","wordBoundaryEnabled":"true"},"outputFormat":"audio-24khz-48kbitrate-mono-mp3"}}}}`
	if err := conn.Write(ctx, websocket.MessageText, []byte(cfg)); err != nil {
		return nil, fmt.Errorf("tts: send config: %w", err)
	}
	ssmlMsg := "X-RequestId:" + randomHex16() + "\r\nContent-Type:application/ssml+xml\r\n" +
		"X-Timestamp:" + ts + "Z\r\nPath:ssml\r\n\r\n" + buildSSML(script)
	if err := conn.Write(ctx, websocket.MessageText, []byte(ssmlMsg)); err != nil {
		return nil, fmt.Errorf("tts: send ssml: %w", err)
	}

	var audio []byte
	var stamps []WordStamp
	for {
		typ, data, err := conn.Read(ctx)
		if err != nil {
			return nil, fmt.Errorf("tts: read: %w", err)
		}
		switch typ {
		case websocket.MessageText:
			headers, body := splitTextMessage(string(data))
			switch headers["Path"] {
			case "audio.metadata":
				ws, err := parseMetadata([]byte(body))
				if err != nil {
					return nil, err
				}
				stamps = append(stamps, ws...)
			case "turn.end":
				if len(audio) == 0 {
					return nil, fmt.Errorf("tts: no audio received")
				}
				if len(stamps) == 0 {
					return nil, fmt.Errorf("tts: no word boundaries received")
				}
				if err := os.WriteFile(outPath, audio, 0o644); err != nil {
					return nil, fmt.Errorf("tts: write %s: %w", outPath, err)
				}
				return stamps, nil
			}
		case websocket.MessageBinary:
			headers, payload, err := parseBinaryMessage(data)
			if err != nil {
				return nil, err
			}
			if headers["Path"] == "audio" {
				audio = append(audio, payload...)
			}
		}
	}
}

func randomHex16() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}
```

`internal/tts/edge_live_test.go`:

```go
package tts

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// Live network test: MOTWR_TTS_LIVE=1 go test ./internal/tts/ -run Live -v
func TestSynthesizeLive(t *testing.T) {
	if os.Getenv("MOTWR_TTS_LIVE") == "" {
		t.Skip("set MOTWR_TTS_LIVE=1 to run the live edge-tts test")
	}
	out := filepath.Join(t.TempDir(), "vo.mp3")
	stamps, err := Synthesize(context.Background(), "Our journey begins in Kochi.", out)
	if err != nil {
		t.Fatalf("Synthesize: %v", err)
	}
	if len(stamps) < 4 {
		t.Fatalf("expected >=4 word stamps, got %d: %+v", len(stamps), stamps)
	}
	for i := 1; i < len(stamps); i++ {
		if stamps[i].Start < stamps[i-1].Start {
			t.Errorf("stamps out of order at %d", i)
		}
	}
	if fi, err := os.Stat(out); err != nil || fi.Size() < 1000 {
		t.Fatalf("mp3 too small or missing: %v", err)
	}
}
```

- [ ] **Step 6: Run all tts tests (pure pass, live runs for real)**

Run: `go test ./internal/tts/ && MOTWR_TTS_LIVE=1 go test ./internal/tts/ -run Live -v`
Expected: pure tests PASS; live test PASS (network required — if it fails with HTTP 403, the `Sec-MS-GEC` clock algorithm or version string needs updating to match current edge-tts; check https://github.com/rany2/edge-tts `src/edge_tts/drm.py`)

- [ ] **Step 7: Commit**

```bash
git add internal/tts/ go.mod go.sum
git commit -m "feat: native edge-tts client with word timestamps"
```

---

### Task 8: Filter graph builder

**Files:**
- Create: `internal/ffmpeg/filtergraph.go`
- Test: `internal/ffmpeg/filtergraph_test.go`

**Interfaces:**
- Consumes: `schedule.Appearance` (Task 4)
- Produces:
  - `type RenderPlan struct { BaseVideo, Logo, Voiceover, Background, BirdSFX, ASSFile, FontsDir, OutPath string; BirdClipPaths []string; Appearances []schedule.Appearance; SpeedFactor, MainDuration float64 }` — `BirdClipPaths[i]` is the webm for `Appearances[i]`
  - `func BuildMainArgs(p RenderPlan) []string` — complete ffmpeg arg list for the main-segment render
  - `func ConcatArgsCopy(listFile, outPath string) []string` and `func ConcatArgsReencode(mainPath, outroPath, outPath string) []string`

- [ ] **Step 1: Write the failing tests**

`internal/ffmpeg/filtergraph_test.go`:

```go
package ffmpeg

import (
	"strings"
	"testing"

	"github.com/lascade/motwr/internal/schedule"
)

func testPlan() RenderPlan {
	return RenderPlan{
		BaseVideo:  "/tmp/base.mp4",
		Logo:       "/a/logo.png",
		Voiceover:  "/tmp/vo.mp3",
		Background: "/a/background/car.mp3",
		BirdSFX:    "/a/bird-sfx.mp3",
		ASSFile:    "/tmp/captions.ass",
		FontsDir:   "/a/fonts",
		OutPath:    "/tmp/main.mp4",
		BirdClipPaths: []string{"/a/bird/2.webm", "/a/bird/0.webm"},
		Appearances: []schedule.Appearance{
			{Clip: 2, Start: 0, End: 4.366},
			{Clip: 0, Start: 14.366, End: 17.399},
		},
		SpeedFactor:  1.25,
		MainDuration: 20.0,
	}
}

func argsJoined(t *testing.T) string {
	t.Helper()
	return strings.Join(BuildMainArgs(testPlan()), " ")
}

func TestBuildMainArgsInputOrder(t *testing.T) {
	s := argsJoined(t)
	// Every bird input must be decoded with libvpx-vp9 to keep alpha (ADR-0002).
	if !strings.Contains(s, "-c:v libvpx-vp9 -i /a/bird/2.webm") ||
		!strings.Contains(s, "-c:v libvpx-vp9 -i /a/bird/0.webm") {
		t.Error("bird inputs must be preceded by -c:v libvpx-vp9")
	}
	if !strings.HasPrefix(s, "-y -i /tmp/base.mp4") {
		t.Errorf("base video must be input 0: %s", s[:60])
	}
	if !strings.Contains(s, "-stream_loop -1 -i /a/background/car.mp3") {
		t.Error("background must loop")
	}
}

func TestBuildMainArgsFilterGraph(t *testing.T) {
	s := argsJoined(t)
	for _, want := range []string{
		"setpts=1.250000*PTS",                    // uniform speed ramp
		"scale=1080:1920",                        // base scaled to output
		"fps=30",                                 //
		"force_original_aspect_ratio=increase,crop=1080:1920", // bird cover-crop
		"enable='between(t,0.000,4.366)'",        // first appearance window
		"enable='between(t,14.366,17.399)'",      // second appearance window
		"eof_action=pass",                        // chain continues after bird ends
		"overlay=main_w-overlay_w-40:40",         // logo top-right
		"subtitles=filename='/tmp/captions.ass':fontsdir='/a/fonts'",
		"volume=1.5",                             // voiceover boost
		"volume=0.3",                             // ambience level
		"volume=0.4",                             // sfx level
		"adelay=delays=14366:all=1",              // 2nd sfx delayed to its bird
		"amix=inputs=4:duration=first:dropout_transition=0:normalize=0",
		"format=yuv420p",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("args missing %q", want)
		}
	}
}

func TestBuildMainArgsEncodeAndDuration(t *testing.T) {
	s := argsJoined(t)
	for _, want := range []string{
		"-map [vout] -map [aout]",
		"-c:v libx264 -preset medium -crf 18 -r 30",
		"-c:a aac -b:a 192k -ar 44100 -ac 2",
		"-t 20.000",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("args missing %q", want)
		}
	}
	args := BuildMainArgs(testPlan())
	if args[len(args)-1] != "/tmp/main.mp4" {
		t.Errorf("last arg must be output path, got %s", args[len(args)-1])
	}
}

func TestConcatArgs(t *testing.T) {
	c := strings.Join(ConcatArgsCopy("/tmp/list.txt", "/tmp/out.mp4"), " ")
	if !strings.Contains(c, "-f concat -safe 0 -i /tmp/list.txt -c copy") {
		t.Errorf("copy concat args wrong: %s", c)
	}
	r := strings.Join(ConcatArgsReencode("/tmp/main.mp4", "/a/outro.mp4", "/tmp/out.mp4"), " ")
	if !strings.Contains(r, "concat=n=2:v=1:a=1") {
		t.Errorf("reencode concat args wrong: %s", r)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/ffmpeg/`
Expected: FAIL — `RenderPlan` undefined

- [ ] **Step 3: Write the implementation**

`internal/ffmpeg/filtergraph.go`:

```go
package ffmpeg

import (
	"fmt"
	"strings"

	"github.com/lascade/motwr/internal/schedule"
)

// RenderPlan is everything BuildMainArgs needs to assemble the single-pass
// main-segment render (ADR-0002).
type RenderPlan struct {
	BaseVideo, Logo, Voiceover, Background, BirdSFX string
	ASSFile, FontsDir, OutPath                      string
	BirdClipPaths                                   []string // per appearance
	Appearances                                     []schedule.Appearance
	SpeedFactor                                     float64 // voiceoverDur / videoDur
	MainDuration                                    float64 // voiceover duration, seconds
}

// BuildMainArgs returns the complete ffmpeg argument list for the main
// segment. Input layout: 0=base, 1..n=bird clips, n+1=logo, n+2=voiceover,
// n+3=background (looped), n+4=bird sfx.
func BuildMainArgs(p RenderPlan) []string {
	n := len(p.Appearances)
	args := []string{"-y", "-i", p.BaseVideo}
	for _, clip := range p.BirdClipPaths {
		// libvpx-vp9 decoder keeps the alpha channel; the native vp9
		// decoder silently drops it (ADR-0002).
		args = append(args, "-c:v", "libvpx-vp9", "-i", clip)
	}
	args = append(args, "-i", p.Logo)
	args = append(args, "-i", p.Voiceover)
	args = append(args, "-stream_loop", "-1", "-i", p.Background)
	args = append(args, "-i", p.BirdSFX)
	logoIdx, voIdx, bgIdx, sfxIdx := n+1, n+2, n+3, n+4

	var g strings.Builder
	// Base video: uniform Speed Ramp, output size, constant frame rate.
	fmt.Fprintf(&g, "[0:v]setpts=%f*PTS,scale=1080:1920,fps=30[v0];", p.SpeedFactor)
	cur := "v0"
	for i, a := range p.Appearances {
		fmt.Fprintf(&g,
			"[%d:v]scale=1080:1920:force_original_aspect_ratio=increase,crop=1080:1920,setpts=PTS-STARTPTS+%.3f/TB[bird%d];",
			i+1, a.Start, i)
		fmt.Fprintf(&g,
			"[%s][bird%d]overlay=0:0:eof_action=pass:enable='between(t,%.3f,%.3f)'[v%d];",
			cur, i, a.Start, a.End, i+1)
		cur = fmt.Sprintf("v%d", i+1)
	}
	fmt.Fprintf(&g, "[%d:v]scale=218:-1[logo];", logoIdx)
	fmt.Fprintf(&g, "[%s][logo]overlay=main_w-overlay_w-40:40[vlogo];", cur)
	fmt.Fprintf(&g, "[vlogo]subtitles=filename=%s:fontsdir=%s,format=yuv420p[vout];",
		escapeFilterArg(p.ASSFile), escapeFilterArg(p.FontsDir))

	// Audio: voiceover first so amix duration=first tracks it.
	fmt.Fprintf(&g, "[%d:a]volume=1.5[avo];", voIdx)
	fmt.Fprintf(&g, "[%d:a]atrim=0:%.3f,volume=0.3,afade=t=out:st=%.3f:d=1.5[abg];",
		bgIdx, p.MainDuration, max(0, p.MainDuration-1.5))
	fmt.Fprintf(&g, "[%d:a]asplit=%d", sfxIdx, n)
	for i := range p.Appearances {
		fmt.Fprintf(&g, "[sfx%d]", i)
	}
	g.WriteString(";")
	for i, a := range p.Appearances {
		l := a.End - a.Start
		fmt.Fprintf(&g,
			"[sfx%d]atrim=0:%.3f,afade=t=out:st=%.3f:d=0.3,volume=0.4,adelay=delays=%d:all=1[asfx%d];",
			i, l, max(0, l-0.3), int(a.Start*1000), i)
	}
	g.WriteString("[avo][abg]")
	for i := range p.Appearances {
		fmt.Fprintf(&g, "[asfx%d]", i)
	}
	fmt.Fprintf(&g, "amix=inputs=%d:duration=first:dropout_transition=0:normalize=0[aout]", n+2)

	args = append(args, "-filter_complex", g.String(),
		"-map", "[vout]", "-map", "[aout]",
		"-c:v", "libx264", "-preset", "medium", "-crf", "18", "-r", "30",
		"-c:a", "aac", "-b:a", "192k", "-ar", "44100", "-ac", "2",
		"-t", fmt.Sprintf("%.3f", p.MainDuration),
		p.OutPath)
	return args
}

// ConcatArgsCopy stream-copies main+outro listed in a concat-demuxer file.
func ConcatArgsCopy(listFile, outPath string) []string {
	return []string{"-y", "-f", "concat", "-safe", "0", "-i", listFile, "-c", "copy", outPath}
}

// ConcatArgsReencode is the fallback when stream-copy fails (e.g. outro
// encode parameters drifted from ours).
func ConcatArgsReencode(mainPath, outroPath, outPath string) []string {
	return []string{"-y", "-i", mainPath, "-i", outroPath,
		"-filter_complex",
		"[0:v]fps=30,scale=1080:1920[v0];[1:v]fps=30,scale=1080:1920[v1];" +
			"[0:a]aresample=44100,aformat=channel_layouts=stereo[a0];" +
			"[1:a]aresample=44100,aformat=channel_layouts=stereo[a1];" +
			"[v0][a0][v1][a1]concat=n=2:v=1:a=1[v][a]",
		"-map", "[v]", "-map", "[a]",
		"-c:v", "libx264", "-preset", "medium", "-crf", "18", "-r", "30",
		"-c:a", "aac", "-b:a", "192k", "-ar", "44100", "-ac", "2",
		outPath}
}

// escapeFilterArg quotes a path for use inside -filter_complex.
func escapeFilterArg(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `'`, `\'`)
	return "'" + s + "'"
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/ffmpeg/`
Expected: PASS

- [ ] **Step 5: Smoke-run the graph against real assets (manual verification)**

```bash
cd /Users/rohittp/Data/Lascade/motwr
# Generate a 9:16 fixture and a voiceover stand-in, plus a minimal ASS file,
# then render one main segment with real assets via scripts/smoke:
ffmpeg -hide_banner -v error -y -f lavfi -i "testsrc=duration=6:size=1080x1920:rate=30" -c:v libx264 -pix_fmt yuv420p /tmp/base.mp4
ffmpeg -hide_banner -v error -y -f lavfi -i "sine=frequency=330:duration=8" -c:a libmp3lame /tmp/vo.mp3
printf '[Script Info]\nScriptType: v4.00+\nPlayResX: 1080\nPlayResY: 1920\n\n[V4+ Styles]\nFormat: Name, Fontname, Fontsize, PrimaryColour, SecondaryColour, OutlineColour, BackColour, Bold, Italic, Underline, StrikeOut, ScaleX, ScaleY, Spacing, Angle, BorderStyle, Outline, Shadow, Alignment, MarginL, MarginR, MarginV, Encoding\nStyle: Caption,Montserrat,42,&H00FFFFFF,&H00FFFFFF,&H66000000,&H66000000,-1,0,0,0,100,100,0,0,3,12,0,8,54,54,0,1\n\n[Events]\nFormat: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\nDialogue: 0,0:00:00.00,0:00:08.00,Caption,,0,0,0,,{\\an5\\pos(540,1250)}SMOKE TEST\n' > /tmp/captions.ass
```

Create `scripts/smoke/main.go` (committed as a dev utility):

```go
// Command smoke renders one main segment using real assets and a synthetic
// base/voiceover, to eyeball the filter graph end-to-end without TTS.
package main

import (
	"context"
	"fmt"
	"math/rand"
	"os"

	"github.com/lascade/motwr/internal/ffmpeg"
	"github.com/lascade/motwr/internal/schedule"
)

func main() {
	apps := schedule.Build(8, []float64{3.033, 4.366, 4.366, 12.9}, 10, rand.New(rand.NewSource(1)))
	clips := make([]string, len(apps))
	for i, a := range apps {
		clips[i] = fmt.Sprintf("assets/bird/%d.webm", a.Clip)
	}
	p := ffmpeg.RenderPlan{
		BaseVideo: "/tmp/base.mp4", Logo: "assets/logo.png",
		Voiceover: "/tmp/vo.mp3", Background: "assets/background/car.mp3",
		BirdSFX: "assets/bird-sfx.mp3", ASSFile: "/tmp/captions.ass",
		FontsDir: "assets/fonts", OutPath: "/tmp/smoke-main.mp4",
		BirdClipPaths: clips, Appearances: apps,
		SpeedFactor: 8.0 / 6.0, MainDuration: 8,
	}
	if err := ffmpeg.Run(context.Background(), ffmpeg.BuildMainArgs(p)); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("rendered /tmp/smoke-main.mp4 — open it and check bird+caption+logo")
}
```

Run: `go run ./scripts/smoke && open /tmp/smoke-main.mp4`
Expected: an 8 s clip — test pattern slowed to 8 s, a bird with transparency at t=0, "SMOKE TEST" pill caption, logo top-right, sine voiceover + car ambience + bird sfx audible. **If the bird shows a black rectangle instead of transparency, the libvpx alpha decode is broken — stop and fix before continuing.**

- [ ] **Step 6: Commit**

```bash
git add internal/ffmpeg/filtergraph.go internal/ffmpeg/filtergraph_test.go scripts/smoke/
git commit -m "feat: single-pass ffmpeg filter graph builder + smoke script"
```

---

### Task 9: CLI wiring, concat, end-to-end

**Files:**
- Create: `cmd/motwr/main.go`
- Create: `internal/ffmpeg/concat.go`
- Create: `cmd/motwr/main_test.go` (E2E, gated)
- Create: `README.md`

**Interfaces:**
- Consumes: everything above
- Produces:
  - `func Concat(ctx context.Context, mainPath, outroPath, outPath string) error` (in `internal/ffmpeg`)
  - binary `motwr` with flags `-job`, `-video`, `-o`, `-assets` (default `./assets`), `-seed` (default 0 = time-based)

- [ ] **Step 1: Write concat implementation**

`internal/ffmpeg/concat.go`:

```go
package ffmpeg

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Concat appends outro after main. Tries stream-copy first (encode params
// match by construction); falls back to re-encode (ADR-0002).
func Concat(ctx context.Context, mainPath, outroPath, outPath string) error {
	list := filepath.Join(filepath.Dir(mainPath), "concat.txt")
	content := fmt.Sprintf("file '%s'\nfile '%s'\n",
		strings.ReplaceAll(mainPath, "'", `'\''`),
		strings.ReplaceAll(outroPath, "'", `'\''`))
	if err := os.WriteFile(list, []byte(content), 0o644); err != nil {
		return fmt.Errorf("concat list: %w", err)
	}
	if err := Run(ctx, ConcatArgsCopy(list, outPath)); err == nil {
		return nil
	}
	return Run(ctx, ConcatArgsReencode(mainPath, outroPath, outPath))
}
```

- [ ] **Step 2: Write the CLI**

`cmd/motwr/main.go`:

```go
// Command motwr renders a branded 1080x1920 route video from a base video
// and a job JSON. See docs/superpowers/specs/2026-07-08-video-encoding-
// pipeline-design.md.
package main

import (
	"context"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/lascade/motwr/internal/captions"
	"github.com/lascade/motwr/internal/ffmpeg"
	"github.com/lascade/motwr/internal/job"
	"github.com/lascade/motwr/internal/schedule"
	"github.com/lascade/motwr/internal/tts"
)

const birdGapSeconds = 10.0

type options struct {
	job, video, out, assets string
	seed                    int64
}

func main() {
	var o options
	flag.StringVar(&o.job, "job", "", "job JSON: local path or http(s) URL (required)")
	flag.StringVar(&o.video, "video", "", "base video mp4, must be 9:16 (required)")
	flag.StringVar(&o.out, "o", "", "output mp4 path (required)")
	flag.StringVar(&o.assets, "assets", "./assets", "assets directory")
	flag.Int64Var(&o.seed, "seed", 0, "RNG seed for bird selection (0 = time-based)")
	flag.Parse()
	if o.job == "" || o.video == "" || o.out == "" {
		flag.Usage()
		os.Exit(2)
	}
	if err := run(context.Background(), o); err != nil {
		fmt.Fprintln(os.Stderr, "motwr:", err)
		os.Exit(1)
	}
}

func step(name string) { fmt.Fprintln(os.Stderr, "==>", name) }

func run(ctx context.Context, o options) error {
	// 1. Load job -----------------------------------------------------------
	step("loading job")
	j, err := job.Load(o.job)
	if err != nil {
		return err
	}

	// 2. Validate environment, assets, and base video ------------------------
	step("validating inputs")
	for _, bin := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(bin); err != nil {
			return fmt.Errorf("%s not found on PATH", bin)
		}
	}
	a := assetPaths(o.assets, j.Vehicle)
	for _, p := range a.all() {
		if _, err := os.Stat(p); err != nil {
			return fmt.Errorf("missing asset: %s", p)
		}
	}
	base, err := ffmpeg.Probe(ctx, o.video)
	if err != nil {
		return err
	}
	if err := ffmpeg.CheckAspect916(base.Width, base.Height); err != nil {
		return err
	}

	// Title must fit before we spend time on TTS.
	antonFont, err := captions.LoadFont(a.fontAnton)
	if err != nil {
		return err
	}
	titleSize, err := captions.FitTitleSize(antonFont, strings.ToUpper(j.Title),
		captions.TitleMaxWidth, captions.TitleStartSize, captions.TitleFloorSize,
		captions.TitleLetterSpacing)
	if err != nil {
		return err
	}

	tmp, err := os.MkdirTemp("", "motwr-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)

	// 3. TTS ------------------------------------------------------------------
	step("generating voiceover (edge-tts)")
	voPath := filepath.Join(tmp, "voiceover.mp3")
	stamps, err := tts.Synthesize(ctx, j.Script, voPath)
	if err != nil {
		return err
	}

	// 4. Durations + speed factor ---------------------------------------------
	step("probing durations")
	vo, err := ffmpeg.Probe(ctx, voPath)
	if err != nil {
		return err
	}
	mainDur := vo.Duration
	speedFactor := mainDur / base.Duration

	// 5. Captions ASS ----------------------------------------------------------
	step("building captions")
	words := make([]captions.Word, len(stamps))
	for i, s := range stamps {
		words[i] = captions.Word{Text: s.Text, Start: s.Start.Seconds(), End: s.End.Seconds()}
	}
	pages := captions.BuildPages(words, captions.MaxPageDur)
	assPath := filepath.Join(tmp, "captions.ass")
	ass := captions.GenerateASS(j.Title, j.Subtitle, titleSize, pages, mainDur)
	if err := os.WriteFile(assPath, []byte(ass), 0o644); err != nil {
		return err
	}

	// 6. Bird schedule ----------------------------------------------------------
	step("scheduling birds")
	clipDurs := make([]float64, len(a.birds))
	for i, p := range a.birds {
		r, err := ffmpeg.Probe(ctx, p)
		if err != nil {
			return err
		}
		clipDurs[i] = r.Duration
	}
	seed := o.seed
	if seed == 0 {
		seed = time.Now().UnixNano()
	}
	apps := schedule.Build(mainDur, clipDurs, birdGapSeconds, rand.New(rand.NewSource(seed)))
	clipPaths := make([]string, len(apps))
	for i, ap := range apps {
		clipPaths[i] = a.birds[ap.Clip]
	}

	// 7. Main render -------------------------------------------------------------
	step("rendering main segment")
	mainPath := filepath.Join(tmp, "main.mp4")
	plan := ffmpeg.RenderPlan{
		BaseVideo: o.video, Logo: a.logo, Voiceover: voPath,
		Background: a.background, BirdSFX: a.birdSFX,
		ASSFile: assPath, FontsDir: a.fontsDir, OutPath: mainPath,
		BirdClipPaths: clipPaths, Appearances: apps,
		SpeedFactor: speedFactor, MainDuration: mainDur,
	}
	if err := ffmpeg.Run(ctx, ffmpeg.BuildMainArgs(plan)); err != nil {
		return err
	}

	// 8. Outro concat --------------------------------------------------------------
	step("appending outro")
	if err := ffmpeg.Concat(ctx, mainPath, a.outro, o.out); err != nil {
		return err
	}
	step("done: " + o.out)
	return nil
}

type assets struct {
	birds                    []string
	background, birdSFX      string
	logo, outro              string
	fontsDir                 string
	fontAnton, fontMontserrat string
}

func (a assets) all() []string {
	return append(append([]string{}, a.birds...),
		a.background, a.birdSFX, a.logo, a.outro, a.fontAnton, a.fontMontserrat)
}

func assetPaths(dir string, v job.Vehicle) assets {
	birds := make([]string, 4)
	for i := range birds {
		birds[i] = filepath.Join(dir, "bird", fmt.Sprintf("%d.webm", i))
	}
	return assets{
		birds:          birds,
		background:     filepath.Join(dir, "background", string(v)+".mp3"),
		birdSFX:        filepath.Join(dir, "bird-sfx.mp3"),
		logo:           filepath.Join(dir, "logo.png"),
		outro:          filepath.Join(dir, "outro.mp4"),
		fontsDir:       filepath.Join(dir, "fonts"),
		fontAnton:      filepath.Join(dir, "fonts", "Anton-Regular.ttf"),
		fontMontserrat: filepath.Join(dir, "fonts", "Montserrat-Bold.ttf"),
	}
}
```

- [ ] **Step 3: Build and verify usage errors**

Run: `go build ./... && go vet ./... && ./motwr 2>&1 || go run ./cmd/motwr 2>&1 | head -5`
Expected: compiles clean; running with no flags prints usage and exits 2.

- [ ] **Step 4: Write the gated E2E test**

`cmd/motwr/main_test.go`:

```go
package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/lascade/motwr/internal/ffmpeg"
)

// Full pipeline: real edge-tts (network) + real ffmpeg render.
// Run with: MOTWR_E2E=1 go test ./cmd/motwr/ -v -timeout 10m
func TestEndToEnd(t *testing.T) {
	if os.Getenv("MOTWR_E2E") == "" {
		t.Skip("set MOTWR_E2E=1 to run the end-to-end render test")
	}
	repoRoot, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()

	base := filepath.Join(dir, "base.mp4")
	cmd := exec.Command("ffmpeg", "-hide_banner", "-v", "error", "-y",
		"-f", "lavfi", "-i", "testsrc=duration=5:size=1080x1920:rate=30",
		"-c:v", "libx264", "-pix_fmt", "yuv420p", base)
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("fixture: %v\n%s", err, b)
	}

	jobPath := filepath.Join(dir, "job.json")
	os.WriteFile(jobPath, []byte(`{"title":"Kochi to Goa","subtitle":"1,200 km by road",
	 "script":"Our journey begins in the beautiful coastal city of Kochi and ends in Goa.",
	 "vehicle":"car"}`), 0o644)

	out := filepath.Join(dir, "out.mp4")
	err = run(context.Background(), options{
		job: jobPath, video: base, out: out,
		assets: filepath.Join(repoRoot, "assets"), seed: 1,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	r, err := ffmpeg.Probe(context.Background(), out)
	if err != nil {
		t.Fatalf("probe output: %v", err)
	}
	if r.Width != 1080 || r.Height != 1920 {
		t.Errorf("output %dx%d, want 1080x1920", r.Width, r.Height)
	}
	// Output = voiceover (~5-8s for that script) + outro (~3s).
	if r.Duration < 6 || r.Duration > 20 {
		t.Errorf("suspicious output duration %.2fs", r.Duration)
	}
	fmt.Println("E2E output at", out, "- open it to verify visually")
}
```

- [ ] **Step 5: Run the E2E test**

Run: `MOTWR_E2E=1 go test ./cmd/motwr/ -v -timeout 10m`
Expected: PASS. Then render once against a real TravelAnimator export if available and **watch the output**: karaoke timing tracks the voice, bird appears with transparency at t=0 and again ~10 s after it leaves, title block + logo persist, ambience under voice, outro plays at the end.

- [ ] **Step 6: Test a rejection path for real**

```bash
ffmpeg -hide_banner -v error -y -f lavfi -i "testsrc=duration=1:size=1920x1080:rate=30" -c:v libx264 -pix_fmt yuv420p /tmp/landscape.mp4
printf '{"title":"T","subtitle":"S","script":"Hello there world","vehicle":"car"}' > /tmp/job.json
go run ./cmd/motwr -job /tmp/job.json -video /tmp/landscape.mp4 -o /tmp/x.mp4; echo "exit=$?"
```

Expected: `motwr: base video must be 9:16 portrait, got 1920x1080` and `exit=1`, with no temp render work done.

- [ ] **Step 7: Write README**

`README.md`:

```markdown
# motwr

Standalone Go CLI that renders a branded 1080×1920 route short from a
TravelAnimator base video + a job JSON: edge-tts voiceover, karaoke captions,
title block, logo, bird overlays, vehicle ambience, speed-ramped to the
narration, brand outro appended.

## Requirements

- Go ≥ 1.22 (build only)
- `ffmpeg` / `ffprobe` on PATH (with libx264, libvpx, libass)
- Network access at render time (edge-tts)

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
```

- [ ] **Step 8: Full test sweep and commit**

Run: `go test ./... && go vet ./...`
Expected: all PASS (gated tests skip without env vars)

```bash
git add cmd/ internal/ffmpeg/concat.go README.md
git commit -m "feat: motwr CLI wiring, outro concat, e2e test, README"
```

---

## Self-review notes (already applied)

- **Spec coverage:** job load file/URL (T1), 9:16 enforcement (T2), fonts + shrink-to-fit with floor error (T3), gap-based 10 s bird schedule with min-appearance guard (T4), ≤1.2 s pages (T5), title block + karaoke gold-word ASS (T6), edge-tts + WordBoundary + retries + script-size cap (T7), single-pass graph with libvpx alpha decode / looped ambience / mix levels / encode-to-match-outro (T8), concat with copy→re-encode fallback + temp-dir cleanup + fail-fast validation order (T9). Out-of-scope items from spec correctly absent.
- **Known accepted deviations** (documented in constraints): caption pill has square corners (ASS BorderStyle=3); caption page display extends to the last word's end rather than hard-cutting at 1.2 s.
- **Type consistency check:** `schedule.Appearance{Clip,Start,End}` used identically in T4/T8/T9; `captions.Word{Text,Start,End}` in T5/T6/T9; `tts.WordStamp` uses `time.Duration` and is converted to seconds in T9 (`.Seconds()`); `RenderPlan.BirdClipPaths[i]` ↔ `Appearances[i]` contract stated in both T8 and T9.
- **Risk callouts for the implementer:** the edge-tts endpoint occasionally changes its DRM (`Sec-MS-GEC`) requirements — if the live test 403s, sync the algorithm with rany2/edge-tts `drm.py`. If `subtitles=` can't find fonts by family name ("Anton", "Montserrat"), verify the TTFs' family names with `fc-scan` and adjust the ASS style Fontname to match.
```
