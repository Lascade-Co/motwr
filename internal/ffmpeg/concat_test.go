package ffmpeg

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os/exec"
	"path/filepath"
	"testing"
)

// makeFixtureDuration renders a tiny lavfi testsrc+sine mp4 with the same
// encode params as makeFixture (probe_test.go), but a caller-chosen
// duration, so main/outro pairs can be concatenated by stream copy.
func makeFixtureDuration(t *testing.T, name string, seconds float64) string {
	t.Helper()
	out := filepath.Join(t.TempDir(), name)
	cmd := exec.Command("ffmpeg", "-hide_banner", "-v", "error", "-y",
		"-f", "lavfi", "-i", fmt.Sprintf("testsrc=duration=%v:size=540x960:rate=30", seconds),
		"-f", "lavfi", "-i", fmt.Sprintf("sine=frequency=440:duration=%v", seconds),
		"-c:v", "libx264", "-pix_fmt", "yuv420p", "-c:a", "aac", "-shortest", out)
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("fixture: %v\n%s", err, b)
	}
	return out
}

func TestConcatHappyPath(t *testing.T) {
	requireFFmpeg(t)
	ctx := context.Background()

	main := makeFixtureDuration(t, "main.mp4", 2)
	outro := makeFixtureDuration(t, "outro.mp4", 1)
	out := filepath.Join(t.TempDir(), "out.mp4")

	if err := Concat(ctx, main, outro, out); err != nil {
		t.Fatalf("Concat: %v", err)
	}

	r, err := Probe(ctx, out)
	if err != nil {
		t.Fatalf("Probe(out): %v", err)
	}
	if math.Abs(r.Duration-3.0) > 0.3 {
		t.Errorf("duration %.3f, want ~3.0 (main 2s + outro 1s)", r.Duration)
	}
}

func TestConcatPropagatesProbeFailureOnMissingInput(t *testing.T) {
	requireFFmpeg(t)
	ctx := context.Background()

	outro := makeFixtureDuration(t, "outro.mp4", 1)
	out := filepath.Join(t.TempDir(), "out.mp4")

	err := Concat(ctx, "/nonexistent/main.mp4", outro, out)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestVerifyConcatDurationOK(t *testing.T) {
	requireFFmpeg(t)
	ctx := context.Background()

	main := makeFixtureDuration(t, "main.mp4", 2)
	outro := makeFixtureDuration(t, "outro.mp4", 1)
	out := filepath.Join(t.TempDir(), "out.mp4")
	if err := Concat(ctx, main, outro, out); err != nil {
		t.Fatalf("Concat: %v", err)
	}

	if err := verifyConcatDuration(ctx, main, outro, out); err != nil {
		t.Errorf("verifyConcatDuration: unexpected error %v", err)
	}
}

func TestVerifyConcatDurationCatchesShortOutput(t *testing.T) {
	requireFFmpeg(t)
	ctx := context.Background()

	main := makeFixtureDuration(t, "main.mp4", 2)
	outro := makeFixtureDuration(t, "outro.mp4", 2)
	// Deliberately short "output": stand in for a concat run that silently
	// dropped the outro segment while still exiting 0.
	short := makeFixtureDuration(t, "short.mp4", 1)

	err := verifyConcatDuration(ctx, main, outro, short)
	if err == nil {
		t.Fatal("expected error for short output")
	}
	if !errors.Is(err, errConcatShort) {
		t.Errorf("expected errConcatShort, got %v", err)
	}
}

// TestConcatMismatchedOutroPlaysFully reproduces the production bug: the
// real outro.mp4 has a different H.264 profile (Main vs High) and video
// track timescale (1/30 vs 1/15360) than the pipeline-encoded main segment.
// Stream-copy concat of such files emits non-monotonic DTS packets: the
// muxer squeezes the whole outro video into a few milliseconds — one flashed
// frame, frozen video, audio continuing. The output's VIDEO stream must
// account for both inputs.
func TestConcatMismatchedOutroPlaysFully(t *testing.T) {
	requireFFmpeg(t)
	ctx := context.Background()

	main := makeFixtureDuration(t, "main.mp4", 2)
	// Outro with mismatched codec parameters, like assets/outro.mp4.
	outro := filepath.Join(t.TempDir(), "outro.mp4")
	cmd := exec.Command("ffmpeg", "-hide_banner", "-v", "error", "-y",
		"-f", "lavfi", "-i", "testsrc=duration=2:size=540x960:rate=30",
		"-f", "lavfi", "-i", "sine=frequency=880:duration=2",
		"-c:v", "libx264", "-profile:v", "main", "-video_track_timescale", "30",
		"-pix_fmt", "yuv420p", "-c:a", "aac", "-shortest", outro)
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("outro fixture: %v\n%s", err, b)
	}
	out := filepath.Join(t.TempDir(), "out.mp4")

	if err := Concat(ctx, main, outro, out); err != nil {
		t.Fatalf("Concat: %v", err)
	}
	r, err := Probe(ctx, out)
	if err != nil {
		t.Fatalf("Probe(out): %v", err)
	}
	if math.Abs(r.VideoDuration-4.0) > 0.3 {
		t.Errorf("video stream duration %.3f, want ~4.0 — outro video was dropped/squeezed", r.VideoDuration)
	}
}

// A concat output whose audio is full-length but whose video stream lost the
// outro (the observed silent-failure mode) must be rejected even though the
// container duration looks fine.
func TestVerifyConcatDurationCatchesBrokenVideoStream(t *testing.T) {
	requireFFmpeg(t)
	ctx := context.Background()

	main := makeFixtureDuration(t, "main.mp4", 2)
	outro := makeFixtureDuration(t, "outro.mp4", 1)
	// Fake "output": 3s of audio (container duration passes) but only 0.2s
	// of video — no -shortest, so the streams keep different lengths.
	broken := filepath.Join(t.TempDir(), "broken.mp4")
	cmd := exec.Command("ffmpeg", "-hide_banner", "-v", "error", "-y",
		"-f", "lavfi", "-i", "testsrc=duration=0.2:size=540x960:rate=30",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=3",
		"-c:v", "libx264", "-pix_fmt", "yuv420p", "-c:a", "aac", broken)
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("broken fixture: %v\n%s", err, b)
	}

	err := verifyConcatDuration(ctx, main, outro, broken)
	if err == nil {
		t.Fatal("expected error for output with truncated video stream")
	}
	if !errors.Is(err, errConcatShort) {
		t.Errorf("expected errConcatShort, got %v", err)
	}
}

func TestVerifyConcatDurationPropagatesInputProbeFailure(t *testing.T) {
	requireFFmpeg(t)
	ctx := context.Background()

	outro := makeFixtureDuration(t, "outro.mp4", 1)
	out := makeFixtureDuration(t, "out.mp4", 1)

	err := verifyConcatDuration(ctx, "/nonexistent/main.mp4", outro, out)
	if err == nil {
		t.Fatal("expected error")
	}
	if errors.Is(err, errConcatShort) {
		t.Error("input probe failure must not be reported as errConcatShort")
	}
}
