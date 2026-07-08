package captions

import (
	"strings"
	"testing"

	"github.com/lascade/motwr/internal/config"
)

const antonPath = "../../assets/fonts/Anton-Regular.ttf"

func TestTextWidthPositiveAndMonotonic(t *testing.T) {
	f, err := LoadFont(antonPath)
	if err != nil {
		t.Fatalf("LoadFont: %v", err)
	}
	w88, err := TextWidth(f, "KOCHI TO GOA", 88, 2)
	if err != nil {
		t.Fatal(err)
	}
	w44, err := TextWidth(f, "KOCHI TO GOA", 44, 2)
	if err != nil {
		t.Fatal(err)
	}
	if w88 <= 0 || w44 <= 0 || w88 <= w44 {
		t.Fatalf("widths not monotonic: w88=%.1f w44=%.1f", w88, w44)
	}
}

func TestFitTitleSizeShortTitleKeepsStartSize(t *testing.T) {
	f, _ := LoadFont(antonPath)
	size, err := FitTitleSize(f, "GOA", 972, 88, 48, 2)
	if err != nil || size != 88 {
		t.Fatalf("size=%v err=%v, want 88", size, err)
	}
}

func TestFitTitleSizeShrinksLongTitle(t *testing.T) {
	f, _ := LoadFont(antonPath)
	size, err := FitTitleSize(f, "SAN FRANCISCO TO NEW YORK CITY", 972, 88, 48, 2)
	if err != nil {
		t.Fatal(err)
	}
	if size >= 88 || size < 48 {
		t.Fatalf("size=%.1f, want in [48,88)", size)
	}
	w, _ := TextWidth(f, "SAN FRANCISCO TO NEW YORK CITY", size, 2)
	if w > 972 {
		t.Fatalf("shrunk text still too wide: %.1f", w)
	}
}

func TestLayoutTitleShortStaysOneLine(t *testing.T) {
	f, _ := LoadFont(antonPath)
	lay, err := LayoutTitle(f, "Kochi to Goa")
	if err != nil {
		t.Fatal(err)
	}
	if len(lay.Lines) != 1 || lay.Lines[0] != "KOCHI TO GOA" || lay.Size != config.TitleStartSize {
		t.Fatalf("short title must stay one line at full size, got %+v", lay)
	}
}

func TestLayoutTitleBreaksBeforeScaling(t *testing.T) {
	f, _ := LoadFont(antonPath)
	// Too wide for one line at 88px, but each half fits: must break, not shrink.
	lay, err := LayoutTitle(f, "San Francisco to New York City")
	if err != nil {
		t.Fatal(err)
	}
	if len(lay.Lines) != 2 {
		t.Fatalf("want 2 lines, got %+v", lay)
	}
	if lay.Size != config.TitleStartSize {
		t.Errorf("breaking should have avoided shrinking, got size %.1f", lay.Size)
	}
	for _, ln := range lay.Lines {
		w, _ := TextWidth(f, ln, lay.Size, config.TitleLetterSpacing)
		if w > config.TitleMaxWidth {
			t.Errorf("line %q width %.1f exceeds max %.0f", ln, w, config.TitleMaxWidth)
		}
	}
	if got := lay.Lines[0] + " " + lay.Lines[1]; got != "SAN FRANCISCO TO NEW YORK CITY" {
		t.Errorf("lines lost or reordered words: %q", got)
	}
}

func TestLayoutTitleSingleWordScales(t *testing.T) {
	f, _ := LoadFont(antonPath)
	// One unbreakable word wider than the frame: shrink on a single line.
	lay, err := LayoutTitle(f, "Supercalifragilisticexpialidocious")
	if err != nil {
		t.Fatal(err)
	}
	if len(lay.Lines) != 1 {
		t.Fatalf("single word cannot break, got %+v", lay)
	}
	if lay.Size >= config.TitleStartSize || lay.Size < config.TitleFloorSize {
		t.Errorf("size %.1f, want shrunk within [floor, start)", lay.Size)
	}
}

func TestLayoutTitleTooLongErrors(t *testing.T) {
	f, _ := LoadFont(antonPath)
	long := strings.TrimSpace(strings.Repeat("PNEUMONOULTRAMICROSCOPICSILICOVOLCANOCONIOSIS ", 3))
	if _, err := LayoutTitle(f, long); err == nil ||
		!strings.Contains(err.Error(), "title too long") {
		t.Fatalf("expected 'title too long' error, got %v", err)
	}
}

func TestFitTitleSizeFloorErrors(t *testing.T) {
	f, _ := LoadFont(antonPath)
	long := strings.Repeat("VERY LONG TITLE ", 8)
	if _, err := FitTitleSize(f, long, 972, 88, 48, 2); err == nil ||
		!strings.Contains(err.Error(), "title too long") {
		t.Fatalf("expected 'title too long' error, got %v", err)
	}
}
