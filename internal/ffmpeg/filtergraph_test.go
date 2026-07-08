package ffmpeg

import (
	"strings"
	"testing"

	"github.com/lascade/motwr/internal/schedule"
)

func testPlan() RenderPlan {
	return RenderPlan{
		BaseVideo:  "/tmp/base.mp4",
		Logo:       "/a/logo.png",
		Voiceover:  "/tmp/vo.mp3",
		Background: "/a/background/car.mp3",
		BirdSFX:    "/a/bird-sfx.mp3",
		ASSFile:    "/tmp/captions.ass",
		FontsDir:   "/a/fonts",
		OutPath:    "/tmp/main.mp4",
		BirdClipPaths: []string{"/a/bird/2.webm", "/a/bird/0.webm"},
		Appearances: []schedule.Appearance{
			{Clip: 2, Start: 0, End: 4.366},
			{Clip: 0, Start: 14.366, End: 17.399},
		},
		SpeedFactor:  1.25,
		MainDuration: 20.0,
	}
}

func argsJoined(t *testing.T) string {
	t.Helper()
	return strings.Join(BuildMainArgs(testPlan()), " ")
}

func TestBuildMainArgsInputOrder(t *testing.T) {
	s := argsJoined(t)
	// Every bird input must be decoded with libvpx-vp9 to keep alpha (ADR-0002).
	if !strings.Contains(s, "-c:v libvpx-vp9 -i /a/bird/2.webm") ||
		!strings.Contains(s, "-c:v libvpx-vp9 -i /a/bird/0.webm") {
		t.Error("bird inputs must be preceded by -c:v libvpx-vp9")
	}
	if !strings.HasPrefix(s, "-y -i /tmp/base.mp4") {
		t.Errorf("base video must be input 0: %s", s[:60])
	}
	if !strings.Contains(s, "-stream_loop -1 -i /a/background/car.mp3") {
		t.Error("background must loop")
	}
}

func TestBuildMainArgsFilterGraph(t *testing.T) {
	s := argsJoined(t)
	for _, want := range []string{
		"setpts=1.250000*PTS",                    // uniform speed ramp
		"scale=1080:1920",                        // base scaled to output
		"fps=30",                                 //
		"force_original_aspect_ratio=increase,crop=1080:1920", // bird cover-crop
		"enable='between(t,0.000,4.366)'",        // first appearance window
		"enable='between(t,14.366,17.399)'",      // second appearance window
		"eof_action=pass",                        // chain continues after bird ends
		"overlay=main_w-overlay_w-40:40",         // logo top-right
		"subtitles=filename='/tmp/captions.ass':fontsdir='/a/fonts'",
		"volume=1.5",                             // voiceover boost
		"volume=0.3",                             // ambience level
		"volume=0.4",                             // sfx level
		"adelay=delays=14366:all=1",              // 2nd sfx delayed to its bird
		"amix=inputs=4:duration=first:dropout_transition=0:normalize=0",
		"format=yuv420p",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("args missing %q", want)
		}
	}
}

func TestBuildMainArgsEncodeAndDuration(t *testing.T) {
	s := argsJoined(t)
	for _, want := range []string{
		"-map [vout] -map [aout]",
		"-c:v libx264 -preset medium -crf 18 -r 30",
		"-c:a aac -b:a 192k -ar 44100 -ac 2",
		"-t 20.000",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("args missing %q", want)
		}
	}
	args := BuildMainArgs(testPlan())
	if args[len(args)-1] != "/tmp/main.mp4" {
		t.Errorf("last arg must be output path, got %s", args[len(args)-1])
	}
}

func TestBuildMainArgsWindowsSubtitlePaths(t *testing.T) {
	// A Windows subtitles path carries a drive-letter colon and backslash
	// separators. Both are special at the subtitles filter's inner option
	// parsing level and must be escaped, or ffmpeg rejects the whole
	// -filter_complex ("Invalid argument").
	p := testPlan()
	p.ASSFile = `C:\Users\jishn\AppData\Local\Temp\motwr-1\captions.ass`
	p.FontsDir = `E:\motwr-test\assets\fonts`
	s := strings.Join(BuildMainArgs(p), " ")

	want := `subtitles=filename='C\:\\Users\\jishn\\AppData\\Local\\Temp\\motwr-1\\captions.ass':` +
		`fontsdir='E\:\\motwr-test\\assets\\fonts',format=yuv420p`
	if !strings.Contains(s, want) {
		t.Errorf("windows subtitles path not escaped for inner filter parsing.\nwant substring: %s\ngot: %s", want, s)
	}
}

func TestConcatArgs(t *testing.T) {
	c := strings.Join(ConcatArgsCopy("/tmp/list.txt", "/tmp/out.mp4"), " ")
	if !strings.Contains(c, "-f concat -safe 0 -i /tmp/list.txt -c copy") {
		t.Errorf("copy concat args wrong: %s", c)
	}
	r := strings.Join(ConcatArgsReencode("/tmp/main.mp4", "/a/outro.mp4", "/tmp/out.mp4"), " ")
	if !strings.Contains(r, "concat=n=2:v=1:a=1") {
		t.Errorf("reencode concat args wrong: %s", r)
	}
}
