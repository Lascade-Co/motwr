// Package schedule computes gap-based Bird Appearance timings: first at t=0,
// each next starting `gap` seconds after the previous clip ends, clips
// playing their natural length (see CONTEXT.md "Bird Appearance").
package schedule

import "math/rand"

const minAppearance = 0.5 // seconds; shorter tails are skipped

type Appearance struct {
	Clip       int     // index into the clip-duration slice passed to Build
	Start, End float64 // seconds within the Main Segment
}

func Build(mainDuration float64, clipDurations []float64, gap float64, rng *rand.Rand) []Appearance {
	var out []Appearance
	t := 0.0
	for mainDuration-t >= minAppearance {
		clip := rng.Intn(len(clipDurations))
		end := t + clipDurations[clip]
		if end > mainDuration {
			end = mainDuration
		}
		out = append(out, Appearance{Clip: clip, Start: t, End: end})
		t = end + gap
	}
	return out
}
