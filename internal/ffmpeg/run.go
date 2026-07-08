package ffmpeg

import (
	"bytes"
	"context"
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
