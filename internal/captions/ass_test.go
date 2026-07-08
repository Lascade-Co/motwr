package captions

import (
	"strings"
	"testing"
)

func testPages() []Page {
	return BuildPages([]Word{
		w("Our", 0.0, 0.3), w("journey", 0.35, 0.8), w("begins", 0.85, 1.1),
		w("in", 1.3, 1.4), w("Kochi", 1.5, 2.0),
	}, MaxPageDur)
}

func TestGenerateASSHeaderAndStyles(t *testing.T) {
	out := GenerateASS("Kochi to Goa", "1,200 km by road", 88, testPages(), 30)
	for _, want := range []string{
		"PlayResX: 1080", "PlayResY: 1920",
		"Style: Title,Anton,88,", "Style: Subtitle,Montserrat,32,",
		"Style: Caption,Montserrat,42,",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q", want)
		}
	}
}

func TestGenerateASSTitleBlock(t *testing.T) {
	out := GenerateASS("Kochi to Goa", "1,200 km by road", 72, testPages(), 30)
	if !strings.Contains(out, "KOCHI TO GOA") {
		t.Error("title not uppercased")
	}
	if !strings.Contains(out, "1,200 KM BY ROAD") {
		t.Error("subtitle not uppercased")
	}
	if !strings.Contains(out, "Style: Title,Anton,72,") {
		t.Error("computed title size not baked into style")
	}
	// Title visible for the whole main segment.
	if !strings.Contains(out, "Dialogue: 0,0:00:00.00,0:00:30.00,Title,") {
		t.Error("title event should span 0 to main duration")
	}
}

func TestGenerateASSKaraokeHighlight(t *testing.T) {
	out := GenerateASS("T", "S", 88, testPages(), 30)
	// Active word gold, inactive restored to white.
	if !strings.Contains(out, `{\1c&H00D7FF&}Our{\1c&HFFFFFF&}`) {
		t.Error("missing gold highlight for first word")
	}
	if !strings.Contains(out, `{\1c&H00D7FF&}journey{\1c&HFFFFFF&}`) {
		t.Error("missing gold highlight for second word")
	}
	// The event during the first word runs 0.00 -> 0.30.
	if !strings.Contains(out, "Dialogue: 0,0:00:00.00,0:00:00.30,Caption,") {
		t.Error("missing first word interval event")
	}
}

func TestGenerateASSGapShowsAllWhite(t *testing.T) {
	out := GenerateASS("T", "S", 88, testPages(), 30)
	// Between "Our"(ends 0.30) and "journey"(starts 0.35) no word is active:
	// there must be an event for 0.30->0.35 whose text has no gold override.
	idx := strings.Index(out, "Dialogue: 0,0:00:00.30,0:00:00.35,Caption,")
	if idx < 0 {
		t.Fatal("missing gap event")
	}
	line := out[idx:]
	line = line[:strings.Index(line, "\n")]
	if strings.Contains(line, `\1c&H00D7FF&`) {
		t.Error("gap event must not highlight any word")
	}
}

func TestGenerateASSSanitizesBraces(t *testing.T) {
	pages := BuildPages([]Word{w("hi{x}", 0, 0.5)}, MaxPageDur)
	out := GenerateASS("T{", "S}", 88, pages, 10)
	if strings.Contains(out, "{x}") || strings.Contains(out, "T{,") {
		t.Error("braces must be sanitized out of user text")
	}
}
