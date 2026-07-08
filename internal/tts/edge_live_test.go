package tts

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// Live network test: MOTWR_TTS_LIVE=1 go test ./internal/tts/ -run Live -v
func TestSynthesizeLive(t *testing.T) {
	if os.Getenv("MOTWR_TTS_LIVE") == "" {
		t.Skip("set MOTWR_TTS_LIVE=1 to run the live edge-tts test")
	}
	out := filepath.Join(t.TempDir(), "vo.mp3")
	stamps, err := Synthesize(context.Background(), "Our journey begins in Kochi.", out)
	if err != nil {
		t.Fatalf("Synthesize: %v", err)
	}
	if len(stamps) < 4 {
		t.Fatalf("expected >=4 word stamps, got %d: %+v", len(stamps), stamps)
	}
	for i := 1; i < len(stamps); i++ {
		if stamps[i].Start < stamps[i-1].Start {
			t.Errorf("stamps out of order at %d", i)
		}
	}
	if fi, err := os.Stat(out); err != nil || fi.Size() < 1000 {
		t.Fatalf("mp3 too small or missing: %v", err)
	}
}
