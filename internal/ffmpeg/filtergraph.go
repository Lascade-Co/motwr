package ffmpeg

import (
	"fmt"
	"math"
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
			i, l, max(0, l-0.3), int(math.Round(a.Start*1000)), i)
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
		"[0:v]fps=30,scale=1080:1920,format=yuv420p[v0];[1:v]fps=30,scale=1080:1920,format=yuv420p[v1];" +
			"[0:a]aresample=44100,aformat=channel_layouts=stereo[a0];" +
			"[1:a]aresample=44100,aformat=channel_layouts=stereo[a1];" +
			"[v0][a0][v1][a1]concat=n=2:v=1:a=1[v][a]",
		"-map", "[v]", "-map", "[a]",
		"-c:v", "libx264", "-preset", "medium", "-crf", "18", "-r", "30",
		"-c:a", "aac", "-b:a", "192k", "-ar", "44100", "-ac", "2",
		outPath}
}

// escapeFilterArg quotes a path for use inside -filter_complex. ffmpeg does
// no escape processing inside '...'; a literal quote needs close-reopen:
// it's → 'it'\''s'.
func escapeFilterArg(s string) string {
	return "'" + strings.ReplaceAll(s, `'`, `'\''`) + "'"
}
