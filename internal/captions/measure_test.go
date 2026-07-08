package captions

import (
	"strings"
	"testing"
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

func TestFitTitleSizeFloorErrors(t *testing.T) {
	f, _ := LoadFont(antonPath)
	long := strings.Repeat("VERY LONG TITLE ", 8)
	if _, err := FitTitleSize(f, long, 972, 88, 48, 2); err == nil ||
		!strings.Contains(err.Error(), "title too long") {
		t.Fatalf("expected 'title too long' error, got %v", err)
	}
}
