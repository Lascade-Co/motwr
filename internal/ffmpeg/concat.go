package ffmpeg

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/lascade/motwr/internal/config"
)

// errConcatShort marks a verifyConcatDuration failure caused by the
// concatenated output being too short (as opposed to a Probe failure on one
// of the inputs). Concat uses errors.Is against it to decide whether
// falling back to the re-encode path can help.
var errConcatShort = errors.New("concat output shorter than expected")

// Concat appends outro after main. The outro is first re-encoded with the
// pipeline's own encoder settings (NormalizeForConcatArgs) so the stream-copy
// concat's same-codec-parameters precondition genuinely holds — the shipped
// outro.mp4 has a different H.264 profile and track timescale, and copy-
// concatenating mismatched streams silently squeezes the outro video into
// milliseconds (frozen frame, audio still playing). Falls back to a full
// re-encode concat (ADR-0002) if the copy path still fails.
//
// The concat demuxer is also known to exit 0 while silently dropping a
// segment it failed to open (observed in E2E with relative paths), so every
// result is verified against Probe(main)+Probe(outro) — per video stream,
// not just container duration — before it's trusted.
func Concat(ctx context.Context, mainPath, outroPath, outPath string) error {
	dir := filepath.Dir(mainPath)

	normOutro := filepath.Join(dir, "outro-normalized.mp4")
	if err := Run(ctx, NormalizeForConcatArgs(outroPath, normOutro)); err != nil {
		return fmt.Errorf("normalize outro: %w", err)
	}

	list := filepath.Join(dir, "concat.txt")
	content := fmt.Sprintf("file '%s'\nfile '%s'\n",
		strings.ReplaceAll(mainPath, "'", `'\''`),
		strings.ReplaceAll(normOutro, "'", `'\''`))
	if err := os.WriteFile(list, []byte(content), 0o644); err != nil {
		return fmt.Errorf("concat list: %w", err)
	}

	if err := Run(ctx, ConcatArgsCopy(list, outPath)); err == nil {
		if verr := verifyConcatDuration(ctx, mainPath, outroPath, outPath); verr == nil {
			return nil
		} else if !errors.Is(verr, errConcatShort) {
			return verr // main/outro couldn't be probed; re-encoding won't help
		}
		// else: copy-concat silently dropped a segment; fall through to re-encode.
	}

	if err := Run(ctx, ConcatArgsReencode(mainPath, outroPath, outPath)); err != nil {
		return err
	}

	if verr := verifyConcatDuration(ctx, mainPath, outroPath, outPath); verr != nil {
		if !errors.Is(verr, errConcatShort) {
			return verr
		}
		return fmt.Errorf("concat: re-encode still produced a short output: %w", verr)
	}
	return nil
}

// verifyConcatDuration probes main, outro, and the concat output, and
// confirms the output accounts for both inputs' durations (within a 0.5s
// tolerance) — checked separately for the container and for the video
// stream. The video-stream check matters: a botched copy-concat can produce
// a full-length audio track (container duration looks fine) while the whole
// outro video collapses into a few milliseconds. Probe failures on main or
// outro are returned as plain errors (not wrapping errConcatShort) since
// re-encoding the same bad inputs won't help; anything wrong with the output
// itself is wrapped in errConcatShort so callers can tell the cases apart.
func verifyConcatDuration(ctx context.Context, mainPath, outroPath, outPath string) error {
	mainR, err := Probe(ctx, mainPath)
	if err != nil {
		return fmt.Errorf("probe main %s: %w", mainPath, err)
	}
	outroR, err := Probe(ctx, outroPath)
	if err != nil {
		return fmt.Errorf("probe outro %s: %w", outroPath, err)
	}

	outR, err := Probe(ctx, outPath)
	if err != nil {
		return fmt.Errorf("%w: probe concat output %s: %v", errConcatShort, outPath, err)
	}

	const tol = config.ConcatDurationTolerance
	want := mainR.Duration + outroR.Duration
	if want-outR.Duration > tol {
		return fmt.Errorf("%w: got %.3fs, want >= %.3fs (main %.3fs + outro %.3fs)",
			errConcatShort, outR.Duration, want-tol, mainR.Duration, outroR.Duration)
	}

	if mainR.VideoDuration > 0 && outroR.VideoDuration > 0 {
		wantVideo := mainR.VideoDuration + outroR.VideoDuration
		if wantVideo-outR.VideoDuration > tol {
			return fmt.Errorf("%w: video stream is %.3fs, want >= %.3fs (main %.3fs + outro %.3fs) — outro video was dropped or time-squeezed",
				errConcatShort, outR.VideoDuration, wantVideo-0.5, mainR.VideoDuration, outroR.VideoDuration)
		}
	}
	return nil
}
