// Command motwr renders a branded 1080x1920 route video from a base video
// and a job JSON. See docs/superpowers/specs/2026-07-08-video-encoding-
// pipeline-design.md.
package main

import (
	"context"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/lascade/motwr/internal/captions"
	"github.com/lascade/motwr/internal/config"
	"github.com/lascade/motwr/internal/ffmpeg"
	"github.com/lascade/motwr/internal/job"
	"github.com/lascade/motwr/internal/schedule"
	"github.com/lascade/motwr/internal/tts"
)

type options struct {
	job, video, out, assets string
	seed                    int64
}

func main() {
	var o options
	flag.StringVar(&o.job, "job", "", "job JSON: local path or http(s) URL (required)")
	flag.StringVar(&o.video, "video", "", "base video mp4, must be 9:16 (required)")
	flag.StringVar(&o.out, "o", "", "output mp4 path (required)")
	flag.StringVar(&o.assets, "assets", "./assets", "assets directory")
	flag.Int64Var(&o.seed, "seed", 0, "RNG seed for bird selection (0 = time-based)")
	flag.Parse()
	if o.job == "" || o.video == "" || o.out == "" {
		flag.Usage()
		os.Exit(2)
	}
	if err := run(context.Background(), o); err != nil {
		fmt.Fprintln(os.Stderr, "motwr:", err)
		os.Exit(1)
	}
}

func step(name string) { fmt.Fprintln(os.Stderr, "==>", name) }

func run(ctx context.Context, o options) error {
	// 1. Load job -----------------------------------------------------------
	step("loading job")
	j, err := job.Load(o.job)
	if err != nil {
		return err
	}

	// 2. Validate environment, assets, and base video ------------------------
	step("validating inputs")
	for _, bin := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(bin); err != nil {
			return fmt.Errorf("%s not found on PATH", bin)
		}
	}
	// Absolutize path flags: the outro-concat list file lives in a temp
	// directory, and ffmpeg's concat demuxer resolves relative entries
	// relative to that list file (not the process CWD) -- relative asset
	// paths would otherwise resolve to the wrong location.
	if o.assets, err = filepath.Abs(o.assets); err != nil {
		return fmt.Errorf("assets path: %w", err)
	}
	if o.video, err = filepath.Abs(o.video); err != nil {
		return fmt.Errorf("video path: %w", err)
	}
	if o.out, err = filepath.Abs(o.out); err != nil {
		return fmt.Errorf("output path: %w", err)
	}
	a := assetPaths(o.assets, j.Vehicle)
	for _, p := range a.all() {
		if _, err := os.Stat(p); err != nil {
			return fmt.Errorf("missing asset: %s", p)
		}
	}
	base, err := ffmpeg.Probe(ctx, o.video)
	if err != nil {
		return err
	}
	if err := ffmpeg.CheckAspect916(base.Width, base.Height); err != nil {
		return err
	}

	// Title must fit before we spend time on TTS. Titles render in the title
	// face, so LayoutTitle measures against that font.
	titleFont, err := captions.LoadFont(a.fontTitle)
	if err != nil {
		return err
	}
	titleLayout, err := captions.LayoutTitle(titleFont, j.Title)
	if err != nil {
		return err
	}

	tmp, err := os.MkdirTemp("", "motwr-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)

	// 3. TTS ------------------------------------------------------------------
	step("generating voiceover (edge-tts)")
	voPath := filepath.Join(tmp, "voiceover.mp3")
	stamps, err := tts.Synthesize(ctx, j.Script, voPath)
	if err != nil {
		return err
	}

	// 4. Durations + speed factor ---------------------------------------------
	step("probing durations")
	vo, err := ffmpeg.Probe(ctx, voPath)
	if err != nil {
		return err
	}
	mainDur := vo.Duration
	speedFactor := mainDur / base.Duration

	// 5. Captions ASS ----------------------------------------------------------
	step("building captions")
	words := make([]captions.Word, len(stamps))
	for i, s := range stamps {
		words[i] = captions.Word{Text: s.Text, Start: s.Start.Seconds(), End: s.End.Seconds()}
	}
	pages := captions.BuildPages(words, config.MaxPageDur)
	assPath := filepath.Join(tmp, "captions.ass")
	ass := captions.GenerateASS(titleLayout, j.Subtitle, pages, mainDur)
	if err := os.WriteFile(assPath, []byte(ass), 0o644); err != nil {
		return err
	}

	// 6. Bird schedule ----------------------------------------------------------
	step("scheduling birds")
	clipDurs := make([]float64, len(a.birds))
	for i, p := range a.birds {
		r, err := ffmpeg.Probe(ctx, p)
		if err != nil {
			return err
		}
		clipDurs[i] = r.Duration
	}
	seed := o.seed
	if seed == 0 {
		seed = time.Now().UnixNano()
	}
	apps := schedule.Build(mainDur, clipDurs, config.BirdGap, rand.New(rand.NewSource(seed)))
	clipPaths := make([]string, len(apps))
	for i, ap := range apps {
		clipPaths[i] = a.birds[ap.Clip]
	}

	// 7. Main render -------------------------------------------------------------
	step("rendering main segment")
	mainPath := filepath.Join(tmp, "main.mp4")
	plan := ffmpeg.RenderPlan{
		BaseVideo: o.video, Logo: a.logo, Voiceover: voPath,
		Background: a.background, BirdSFX: a.birdSFX,
		ASSFile: assPath, FontsDir: a.fontsDir, OutPath: mainPath,
		BirdClipPaths: clipPaths, Appearances: apps,
		SpeedFactor: speedFactor, MainDuration: mainDur,
	}
	if err := ffmpeg.Run(ctx, ffmpeg.BuildMainArgs(plan)); err != nil {
		return err
	}

	// 8. Outro concat --------------------------------------------------------------
	step("appending outro")
	if err := ffmpeg.Concat(ctx, mainPath, a.outro, o.out); err != nil {
		return err
	}

	// 9. Delete the Base Video ------------------------------------------------------
	// The base video is a single-use input (regenerated per job); remove it
	// now that the final video exists. Guard against deleting the output, and
	// only delete once the output is confirmed written. A failed delete is a
	// warning, not an error -- the render already succeeded.
	if fiOut, statErr := os.Stat(o.out); statErr == nil && fiOut.Size() > 0 {
		if fiVid, statVidErr := os.Stat(o.video); statVidErr == nil {
			if !os.SameFile(fiOut, fiVid) {
				if rmErr := os.Remove(o.video); rmErr != nil {
					fmt.Fprintln(os.Stderr, "motwr: warning: could not delete base video:", rmErr)
				} else {
					step("deleted base video: " + o.video)
				}
			}
		}
	}

	step("done: " + o.out)
	return nil
}

type assets struct {
	birds               []string
	background, birdSFX string
	logo, outro         string
	fontsDir            string
	// fontTitle is the title face (also measured by LayoutTitle); fontCaption
	// and fontMontserrat are loaded by libass from fontsDir at render time.
	fontTitle, fontCaption, fontMontserrat string
}

func (a assets) all() []string {
	return append(append([]string{}, a.birds...),
		a.background, a.birdSFX, a.logo, a.outro,
		a.fontTitle, a.fontCaption, a.fontMontserrat)
}

func assetPaths(dir string, v job.Vehicle) assets {
	birds := make([]string, config.BirdClipCount)
	for i := range birds {
		birds[i] = filepath.Join(dir, "bird", fmt.Sprintf("%d.webm", i))
	}
	return assets{
		birds:          birds,
		background:     filepath.Join(dir, "background", string(v)+".mp3"),
		birdSFX:        filepath.Join(dir, "bird-sfx.mp3"),
		logo:           filepath.Join(dir, "logo.png"),
		outro:          filepath.Join(dir, "outro.mp4"),
		fontsDir:       filepath.Join(dir, "fonts"),
		fontTitle:      filepath.Join(dir, "fonts", "Anton-Regular.ttf"),
		fontCaption:    filepath.Join(dir, "fonts", "Poppins-Caption.ttf"),
		fontMontserrat: filepath.Join(dir, "fonts", "Montserrat-Bold.ttf"),
	}
}
