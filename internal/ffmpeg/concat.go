package ffmpeg

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// errConcatShort marks a verifyConcatDuration failure caused by the
// concatenated output being too short (as opposed to a Probe failure on one
// of the inputs). Concat uses errors.Is against it to decide whether
// falling back to the re-encode path can help.
var errConcatShort = errors.New("concat output shorter than expected")

// Concat appends outro after main. Tries stream-copy first (encode params
// match by construction); falls back to re-encode (ADR-0002).
//
// The concat demuxer is known to exit 0 while silently dropping a segment
// it failed to open (observed in E2E with relative paths), so a successful
// run is verified by comparing the output's duration against
// Probe(main)+Probe(outro) before it's trusted.
func Concat(ctx context.Context, mainPath, outroPath, outPath string) error {
	list := filepath.Join(filepath.Dir(mainPath), "concat.txt")
	content := fmt.Sprintf("file '%s'\nfile '%s'\n",
		strings.ReplaceAll(mainPath, "'", `'\''`),
		strings.ReplaceAll(outroPath, "'", `'\''`))
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
// tolerance). Probe failures on main or outro are returned as plain errors
// (not wrapping errConcatShort) since re-encoding the same bad inputs won't
// help; anything wrong with the output itself (unprobeable, or too short)
// is wrapped in errConcatShort so callers can tell the two cases apart.
func verifyConcatDuration(ctx context.Context, mainPath, outroPath, outPath string) error {
	mainR, err := Probe(ctx, mainPath)
	if err != nil {
		return fmt.Errorf("probe main %s: %w", mainPath, err)
	}
	outroR, err := Probe(ctx, outroPath)
	if err != nil {
		return fmt.Errorf("probe outro %s: %w", outroPath, err)
	}
	want := mainR.Duration + outroR.Duration

	outR, err := Probe(ctx, outPath)
	if err != nil {
		return fmt.Errorf("%w: probe concat output %s: %v", errConcatShort, outPath, err)
	}
	if want-outR.Duration > 0.5 {
		return fmt.Errorf("%w: got %.3fs, want >= %.3fs (main %.3fs + outro %.3fs)",
			errConcatShort, outR.Duration, want-0.5, mainR.Duration, outroR.Duration)
	}
	return nil
}
