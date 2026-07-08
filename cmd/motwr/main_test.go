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
	// The VIDEO stream must be as long as the container: a botched outro
	// concat leaves full-length audio but a video track that ends at the
	// main segment (frozen last frame while the outro audio plays).
	if r.Duration-r.VideoDuration > 0.5 {
		t.Errorf("video stream %.2fs much shorter than container %.2fs — outro video missing",
			r.VideoDuration, r.Duration)
	}
	fmt.Println("E2E output at", out, "- open it to verify visually")
}
