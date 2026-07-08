// Command smoke renders one main segment using real assets and a synthetic
// base/voiceover, to eyeball the filter graph end-to-end without TTS.
package main

import (
	"context"
	"fmt"
	"math/rand"
	"os"

	"github.com/lascade/motwr/internal/ffmpeg"
	"github.com/lascade/motwr/internal/schedule"
)

func main() {
	apps := schedule.Build(8, []float64{3.033, 4.366, 4.366, 12.9}, 10, rand.New(rand.NewSource(1)))
	clips := make([]string, len(apps))
	for i, a := range apps {
		clips[i] = fmt.Sprintf("assets/bird/%d.webm", a.Clip)
	}
	p := ffmpeg.RenderPlan{
		BaseVideo: "/tmp/base.mp4", Logo: "assets/logo.png",
		Voiceover: "/tmp/vo.mp3", Background: "assets/background/car.mp3",
		BirdSFX: "assets/bird-sfx.mp3", ASSFile: "/tmp/captions.ass",
		FontsDir: "assets/fonts", OutPath: "/tmp/smoke-main.mp4",
		BirdClipPaths: clips, Appearances: apps,
		SpeedFactor: 8.0 / 6.0, MainDuration: 8,
	}
	if err := ffmpeg.Run(context.Background(), ffmpeg.BuildMainArgs(p)); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("rendered /tmp/smoke-main.mp4 — open it and check bird+caption+logo")
}
