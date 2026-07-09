package captions

import (
	"strings"
	"testing"

	"github.com/lascade/motwr/internal/config"
)

func oneLine(text string, size float64) TitleLayout {
	return TitleLayout{Lines: []string{text}, Size: size}
}

func testPages() []Page {
	return BuildPages([]Word{
		w("Our", 0.0, 0.3), w("journey", 0.35, 0.8), w("begins", 0.85, 1.1),
		w("in", 1.3, 1.4), w("Kochi", 1.5, 2.0),
	}, config.MaxPageDur)
}

func TestGenerateASSHeaderAndStyles(t *testing.T) {
	out := GenerateASS(oneLine("Kochi to Goa", config.TitleStartSize), "1,200 km by road", testPages(), 30)
	for _, want := range []string{
		"PlayResX: 1080", "PlayResY: 1920", "WrapStyle: 1",
		"Style: Title,Anton,170,",
		"Style: Subtitle,Montserrat,56,",
		"Style: Caption,Poppins Caption,66,",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q", want)
		}
	}
	// Caption is white on a black outline (no box). The Poppins face is
	// already bold+italic, so the style flags stay Bold=0,Italic=0.
	if !strings.Contains(out, "Style: Caption,Poppins Caption,66,&H00FFFFFF,&H00FFFFFF,&H00000000,&H00000000,0,0,") {
		t.Error("caption style not white/outlined without a box")
	}
}

func TestGenerateASSRendersLinesAndUppercasesSubtitle(t *testing.T) {
	// GenerateASS renders title lines verbatim (LayoutTitle has already
	// uppercased them) and uppercases the subtitle itself.
	out := GenerateASS(oneLine("KEY LARGO TO KEY WEST", config.TitleStartSize), "the highway that drives", testPages(), 30)
	if !strings.Contains(out, "KEY LARGO TO KEY WEST") {
		t.Error("title line missing")
	}
	if !strings.Contains(out, "THE HIGHWAY THAT DRIVES") {
		t.Error("subtitle must be uppercased")
	}
	// Title visible for the whole main segment.
	if !strings.Contains(out, "Dialogue: 0,0:00:00.00,0:00:30.00,Title,") {
		t.Error("title event should span 0 to main duration")
	}
}

func TestGenerateASSOneLineTitlePositions(t *testing.T) {
	out := GenerateASS(oneLine("KOCHI TO GOA", 100), "s", testPages(), 30)
	// One top-anchored line at TitleY.
	if !strings.Contains(out, `{\an8\pos(540,200)}KOCHI TO GOA`) {
		t.Error("title line not placed at TitleY")
	}
	// Subtitle below one line: 200 + 100*0.75 + 20 = 295.
	if !strings.Contains(out, `\pos(540,295)`) {
		t.Error("subtitle not positioned below a one-line title")
	}
}

func TestGenerateASSTwoLineTitle(t *testing.T) {
	lay := TitleLayout{Lines: []string{"KEY LARGO TO", "KEY WEST"}, Size: 88}
	out := GenerateASS(lay, "route 66", testPages(), 30)
	// Each line is its own positioned event (no \N join).
	if strings.Contains(out, `KEY LARGO TO\NKEY WEST`) {
		t.Error("title lines must be separate events, not joined with \\N")
	}
	if !strings.Contains(out, `{\an8\pos(540,200)}KEY LARGO TO`) {
		t.Error("first title line missing at TitleY")
	}
	// Second line advances by 88*0.75 = 66 -> 266.
	if !strings.Contains(out, `{\an8\pos(540,266)}KEY WEST`) {
		t.Error("second title line not advanced by one line height")
	}
	// Subtitle below two lines: 200 + 88*0.75*2 + 20 = 352.
	if !strings.Contains(out, `\pos(540,352)`) {
		t.Error("subtitle not pushed below the second title line")
	}
}

func TestGenerateASSDynamicCaptions(t *testing.T) {
	out := GenerateASS(oneLine("T", 170), "S", testPages(), 30)
	// Active word is highlighted gold, then restored to white.
	if !strings.Contains(out, `{\1c&H00D7FF&}Our{\1c&HFFFFFF&}`) {
		t.Error("missing gold highlight for first word")
	}
	if !strings.Contains(out, `{\1c&H00D7FF&}journey{\1c&HFFFFFF&}`) {
		t.Error("missing gold highlight for second word")
	}
	// The event while the first word is spoken runs 0.00 -> 0.30 at CaptionY.
	if !strings.Contains(out, `Dialogue: 0,0:00:00.00,0:00:00.30,Caption,,0,0,0,,{\an5\pos(540,1210)}`) {
		t.Error("missing first-word interval event")
	}
	// The inter-word gap 0.30 -> 0.35 highlights nothing.
	idx := strings.Index(out, "Dialogue: 0,0:00:00.30,0:00:00.35,Caption,")
	if idx < 0 {
		t.Fatal("missing gap event")
	}
	if line := out[idx : idx+strings.Index(out[idx:], "\n")]; strings.Contains(line, `\1c&H00D7FF&`) {
		t.Error("gap event must not highlight any word")
	}
}

func TestGenerateASSSanitizesBraces(t *testing.T) {
	pages := BuildPages([]Word{w("hi{x}", 0, 0.5)}, config.MaxPageDur)
	out := GenerateASS(oneLine("T{", 142), "S}", pages, 10)
	if strings.Contains(out, "{x}") || strings.Contains(out, "T{") {
		t.Error("braces must be sanitized out of user text")
	}
}
