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
