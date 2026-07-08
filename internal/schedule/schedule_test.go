package schedule

import (
	"math/rand"
	"testing"
)

var clips = []float64{3.033, 4.366, 4.366, 12.9}

func TestGapBasedNoOverlap(t *testing.T) {
	apps := Build(60, clips, 10, rand.New(rand.NewSource(1)))
	if len(apps) == 0 {
		t.Fatal("no appearances")
	}
	if apps[0].Start != 0 {
		t.Errorf("first appearance must start at t=0, got %.3f", apps[0].Start)
	}
	for i, a := range apps {
		if a.Clip < 0 || a.Clip >= len(clips) {
			t.Errorf("appearance %d: bad clip %d", i, a.Clip)
		}
		if a.End <= a.Start {
			t.Errorf("appearance %d: End %.3f <= Start %.3f", i, a.End, a.Start)
		}
		if i > 0 {
			wantStart := apps[i-1].End + 10
			if a.Start != wantStart {
				t.Errorf("appearance %d: Start %.3f, want prev End+10 = %.3f", i, a.Start, wantStart)
			}
		}
	}
}

func TestClipTrimmedAtMainEnd(t *testing.T) {
	apps := Build(60, clips, 10, rand.New(rand.NewSource(1)))
	last := apps[len(apps)-1]
	if last.End > 60 {
		t.Errorf("last appearance End %.3f exceeds main duration 60", last.End)
	}
}

func TestSkipsTinyTailAppearance(t *testing.T) {
	// Main duration leaves only 0.3s after the first clip + gap: no 2nd bird.
	first := clips[0] // seeded rng with 1 clip choice space
	apps := Build(first+10+0.3, []float64{first}, 10, rand.New(rand.NewSource(1)))
	if len(apps) != 1 {
		t.Fatalf("expected exactly 1 appearance, got %d", len(apps))
	}
}

func TestDeterministicForSeed(t *testing.T) {
	a := Build(120, clips, 10, rand.New(rand.NewSource(42)))
	b := Build(120, clips, 10, rand.New(rand.NewSource(42)))
	if len(a) != len(b) {
		t.Fatalf("lengths differ: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("appearance %d differs: %+v vs %+v", i, a[i], b[i])
		}
	}
}
