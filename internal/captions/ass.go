package captions

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/lascade/motwr/internal/config"
)

// ASS inline colour overrides for dynamic caption highlighting.
const (
	goldInline  = `{\1c&H` + config.GoldBGR + `&}`
	whiteInline = `{\1c&HFFFFFF&}`
)

// GenerateASS renders the full subtitle file: Title Block events (one per
// title line plus the subtitle) and dynamic word-highlighted Caption events. All
// layout numbers come from internal/config.
func GenerateASS(title TitleLayout, subtitle string, pages []Page, mainDuration float64) string {
	centerX := config.OutputWidth / 2

	var b strings.Builder
	b.WriteString("[Script Info]\n")
	b.WriteString("ScriptType: v4.00+\n")
	fmt.Fprintf(&b, "PlayResX: %d\nPlayResY: %d\n", config.OutputWidth, config.OutputHeight)
	// WrapStyle 1: greedy end-of-line wrapping (used only by captions; title
	// lines are pre-broken by LayoutTitle and each fits its margin).
	b.WriteString("WrapStyle: 1\nScaledBorderAndShadow: yes\n\n")

	b.WriteString("[V4+ Styles]\n")
	b.WriteString("Format: Name, Fontname, Fontsize, PrimaryColour, SecondaryColour, OutlineColour, BackColour, Bold, Italic, Underline, StrikeOut, ScaleX, ScaleY, Spacing, Angle, BorderStyle, Outline, Shadow, Alignment, MarginL, MarginR, MarginV, Encoding\n")
	// Title: rounded Fredoka, white with a heavy black outline + drop shadow.
	// Alignment 8 = top-center.
	fmt.Fprintf(&b, "Style: Title,%s,%s,&H00FFFFFF,&H00FFFFFF,&H00000000,&H00000000,0,0,0,0,100,100,%g,0,1,%g,%g,8,%s,%s,0,1\n",
		config.TitleFont, trimFloat(title.Size), config.TitleLetterSpacing,
		config.TitleOutline, config.TitleShadow, trimFloat(config.TitleMargin), trimFloat(config.TitleMargin))
	// Subtitle: Montserrat Bold, gold, uppercase.
	fmt.Fprintf(&b, "Style: Subtitle,Montserrat,%s,&H00%s,&H00%s,&H00000000,&H00000000,-1,0,0,0,100,100,3,0,1,%g,0,8,%s,%s,0,1\n",
		trimFloat(config.SubtitleSize), config.GoldBGR, config.GoldBGR,
		config.SubtitleOutline, trimFloat(config.TitleMargin), trimFloat(config.TitleMargin))
	// Caption: Poppins (already bold + italic in the font), white, outlined,
	// no box. Alignment 5 = middle-center; long pages wrap within the margin.
	fmt.Fprintf(&b, "Style: Caption,%s,%s,&H00FFFFFF,&H00FFFFFF,&H00000000,&H00000000,0,0,0,0,100,100,%g,0,1,%g,%g,5,%s,%s,0,1\n\n",
		config.CaptionFont, trimFloat(config.CaptionSize), config.CaptionLetterSpacing,
		config.CaptionOutline, config.CaptionShadow, trimFloat(config.CaptionMargin), trimFloat(config.CaptionMargin))

	b.WriteString("[Events]\n")
	b.WriteString("Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\n")

	end := assTime(mainDuration)
	// Title Block: each line is its own top-anchored event so the vertical
	// advance is exactly title.Size*TitleLineHeight (tight, per the reference).
	advance := title.Size * config.TitleLineHeight
	for i, ln := range title.Lines {
		y := config.TitleY + advance*float64(i)
		fmt.Fprintf(&b, "Dialogue: 0,0:00:00.00,%s,Title,,0,0,0,,{\\an8\\pos(%d,%s)}%s\n",
			end, centerX, trimFloat(y), sanitize(ln))
	}
	subY := config.TitleY + advance*float64(len(title.Lines)) + config.SubtitleGap
	fmt.Fprintf(&b, "Dialogue: 0,0:00:00.00,%s,Subtitle,,0,0,0,,{\\an8\\pos(%d,%s)}%s\n",
		end, centerX, trimFloat(subY), sanitize(strings.ToUpper(subtitle)))

	// Dynamic caption events: the whole page stays on screen while the word
	// being spoken is highlighted gold (the rest white), sliced at every word
	// start/end so the highlight tracks the voiceover.
	for i, p := range pages {
		dispEnd := p.Words[len(p.Words)-1].End
		if next := i + 1; next < len(pages) && pages[next].Start() < dispEnd {
			dispEnd = pages[next].Start()
		}
		if dispEnd > mainDuration {
			dispEnd = mainDuration
		}
		for _, iv := range pageIntervals(p, dispEnd) {
			fmt.Fprintf(&b, "Dialogue: 0,%s,%s,Caption,,0,0,0,,{\\an5\\pos(%d,%s)}%s\n",
				assTime(iv.start), assTime(iv.end), centerX, trimFloat(config.CaptionY), pageText(p, iv.active))
		}
	}
	return b.String()
}

type interval struct {
	start, end float64
	active     int // index into page words, -1 = none (inter-word gap)
}

// pageIntervals slices [pageStart, dispEnd] at every word start/end so each
// slice has a constant active word (or none, during inter-word gaps).
func pageIntervals(p Page, dispEnd float64) []interval {
	bounds := []float64{p.Start(), dispEnd}
	for _, w := range p.Words {
		for _, t := range []float64{w.Start, w.End} {
			if t > p.Start() && t < dispEnd {
				bounds = append(bounds, t)
			}
		}
	}
	sort.Float64s(bounds)
	var out []interval
	for i := 0; i+1 < len(bounds); i++ {
		s, e := bounds[i], bounds[i+1]
		if e-s < 0.001 {
			continue
		}
		active := -1
		for wi, w := range p.Words {
			if w.Start <= s+0.0005 && w.End > s+0.0005 {
				active = wi
				break
			}
		}
		out = append(out, interval{start: s, end: e, active: active})
	}
	return out
}

// pageText renders a page's words, the active one gold and the rest white.
func pageText(p Page, active int) string {
	parts := make([]string, len(p.Words))
	for i, w := range p.Words {
		t := sanitize(w.Text)
		if i == active {
			t = goldInline + t + whiteInline
		}
		parts[i] = t
	}
	return strings.Join(parts, " ")
}

// sanitize strips ASS override syntax from user-supplied text.
func sanitize(s string) string {
	s = strings.ReplaceAll(s, "{", "(")
	s = strings.ReplaceAll(s, "}", ")")
	s = strings.ReplaceAll(s, "\n", " ")
	return s
}

func assTime(t float64) string {
	if t < 0 {
		t = 0
	}
	cs := int(math.Round(t * 100))
	h := cs / 360000
	cs %= 360000
	m := cs / 6000
	cs %= 6000
	return fmt.Sprintf("%d:%02d:%02d.%02d", h, m, cs/100, cs%100)
}

// trimFloat renders 88 not 88.000000, but keeps 72.5.
func trimFloat(f float64) string {
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.2f", f), "0"), ".")
}
