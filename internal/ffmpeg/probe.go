// Package ffmpeg wraps ffprobe/ffmpeg invocations and builds the render
// filter graph.
package ffmpeg

import (
	"context"
	"encoding/json"
	"errors"
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

func exitStderr(err error) string {
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return tail(string(ee.Stderr), 500)
	}
	return ""
}

func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return "…" + s[len(s)-n:]
}
