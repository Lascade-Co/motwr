package ffmpeg

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/lascade/motwr/internal/config"
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
	fmt.Fprintf(&g, "[0:v]setpts=%f*PTS,scale=%d:%d,fps=%d[v0];",
		p.SpeedFactor, config.OutputWidth, config.OutputHeight, config.OutputFPS)
	cur := "v0"
	for i, a := range p.Appearances {
		fmt.Fprintf(&g,
			"[%d:v]scale=%d:%d:force_original_aspect_ratio=increase,crop=%d:%d,setpts=PTS-STARTPTS+%.3f/TB[bird%d];",
			i+1, config.OutputWidth, config.OutputHeight, config.OutputWidth, config.OutputHeight, a.Start, i)
		fmt.Fprintf(&g,
			"[%s][bird%d]overlay=0:0:eof_action=pass:enable='between(t,%.3f,%.3f)'[v%d];",
			cur, i, a.Start, a.End, i+1)
		cur = fmt.Sprintf("v%d", i+1)
	}
	fmt.Fprintf(&g, "[%d:v]scale=%d:-1[logo];", logoIdx, config.LogoWidth)
	fmt.Fprintf(&g, "[%s][logo]overlay=main_w-overlay_w-%d:%d[vlogo];",
		cur, config.LogoPadding, config.LogoPadding)
	fmt.Fprintf(&g, "[vlogo]subtitles=filename=%s:fontsdir=%s,format=yuv420p[vout];",
		escapeSubtitlesPath(p.ASSFile), escapeSubtitlesPath(p.FontsDir))

	// Audio: voiceover first so amix duration=first tracks it.
	fmt.Fprintf(&g, "[%d:a]volume=%g[avo];", voIdx, config.VoiceoverVolume)
	fmt.Fprintf(&g, "[%d:a]atrim=0:%.3f,volume=%g,afade=t=out:st=%.3f:d=%g[abg];",
		bgIdx, p.MainDuration, config.BackgroundVolume,
		max(0, p.MainDuration-config.BackgroundFadeOut), config.BackgroundFadeOut)
	fmt.Fprintf(&g, "[%d:a]asplit=%d", sfxIdx, n)
	for i := range p.Appearances {
		fmt.Fprintf(&g, "[sfx%d]", i)
	}
	g.WriteString(";")
	for i, a := range p.Appearances {
		l := a.End - a.Start
		fmt.Fprintf(&g,
			"[sfx%d]atrim=0:%.3f,afade=t=out:st=%.3f:d=%g,volume=%g,adelay=delays=%d:all=1[asfx%d];",
			i, l, max(0, l-config.BirdSFXFadeOut), config.BirdSFXFadeOut,
			config.BirdSFXVolume, int(math.Round(a.Start*1000)), i)
	}
	g.WriteString("[avo][abg]")
	for i := range p.Appearances {
		fmt.Fprintf(&g, "[asfx%d]", i)
	}
	fmt.Fprintf(&g, "amix=inputs=%d:duration=first:dropout_transition=0:normalize=0[aout]", n+2)

	args = append(args, "-filter_complex", g.String(),
		"-map", "[vout]", "-map", "[aout]")
	args = append(args, encodeArgs()...)
	args = append(args,
		"-t", fmt.Sprintf("%.3f", p.MainDuration),
		p.OutPath)
	return args
}

// encodeArgs are the output codec settings shared by every segment the
// pipeline produces. Segments concatenated by stream copy must come from
// the exact same encoder configuration (ADR-0002) — a profile or track
// timescale mismatch makes the concat demuxer emit non-monotonic timestamps
// that squeeze one segment's video into milliseconds.
func encodeArgs() []string {
	return []string{
		"-c:v", "libx264", "-preset", config.VideoPreset,
		"-crf", strconv.Itoa(config.VideoCRF), "-r", strconv.Itoa(config.OutputFPS),
		"-c:a", "aac", "-b:a", config.AudioBitrate,
		"-ar", strconv.Itoa(config.AudioSampleRate), "-ac", strconv.Itoa(config.AudioChannels),
	}
}

// normalizeFilters returns the video/audio filter strings that coerce any
// input into the pipeline's output geometry and audio format.
func normalizeFilters() (vf, af string) {
	vf = fmt.Sprintf("scale=%d:%d,fps=%d,format=yuv420p",
		config.OutputWidth, config.OutputHeight, config.OutputFPS)
	af = fmt.Sprintf("aresample=%d,aformat=channel_layouts=stereo", config.AudioSampleRate)
	return vf, af
}

// NormalizeForConcatArgs re-encodes src with the pipeline's own encoder
// settings and output geometry so it can be stream-copy-concatenated with a
// pipeline-rendered segment. Used on the outro, whose source encode (H.264
// Main profile, 1/30 timescale) does not match ours.
func NormalizeForConcatArgs(srcPath, outPath string) []string {
	vf, af := normalizeFilters()
	args := []string{"-y", "-i", srcPath, "-vf", vf, "-af", af}
	args = append(args, encodeArgs()...)
	return append(args, outPath)
}

// ConcatArgsCopy stream-copies main+outro listed in a concat-demuxer file.
func ConcatArgsCopy(listFile, outPath string) []string {
	return []string{"-y", "-f", "concat", "-safe", "0", "-i", listFile, "-c", "copy", outPath}
}

// ConcatArgsReencode is the fallback when stream-copy fails (e.g. outro
// encode parameters drifted from ours).
func ConcatArgsReencode(mainPath, outroPath, outPath string) []string {
	vf, af := normalizeFilters()
	args := []string{"-y", "-i", mainPath, "-i", outroPath,
		"-filter_complex",
		fmt.Sprintf("[0:v]%s[v0];[1:v]%s[v1];[0:a]%s[a0];[1:a]%s[a1];"+
			"[v0][a0][v1][a1]concat=n=2:v=1:a=1[v][a]", vf, vf, af, af),
		"-map", "[v]", "-map", "[a]"}
	args = append(args, encodeArgs()...)
	return append(args, outPath)
}

// escapeFilterArg quotes a path for use inside -filter_complex. ffmpeg does
// no escape processing inside '...'; a literal quote needs close-reopen:
// it's → 'it'\''s'.
func escapeFilterArg(s string) string {
	return "'" + strings.ReplaceAll(s, `'`, `'\''`) + "'"
}

// escapeSubtitlesPath quotes a path for use as a value of the subtitles
// filter (filename=, fontsdir=). This needs one more level of escaping than
// escapeFilterArg: a filter's option list is split on ':' at an inner parsing
// level whose escape character is '\'. Outer '...' quoting protects the
// filtergraph-level specials ([],;) but NOT this inner split, so a Windows
// drive-letter colon (C:\...) is misread as an option separator and the path
// separators are consumed as escapes. We escape '\' and ':' for that inner
// level, then wrap the result in escapeFilterArg's outer quotes. POSIX paths
// carry neither character, so their output is unchanged.
func escapeSubtitlesPath(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `:`, `\:`)
	return escapeFilterArg(s)
}
