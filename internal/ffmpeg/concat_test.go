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
