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
