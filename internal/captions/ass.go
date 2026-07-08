package captions

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// Layout constants (PlayRes units == output pixels, 1080x1920).
const (
	TitleStartSize     = 88.0
	TitleFloorSize     = 48.0
	TitleMaxWidth      = 972.0 // 90% of 1080
	TitleLetterSpacing = 2.0
	MaxPageDur         = 1.2

	titleY      = 200.0
	subtitleGap = 18.0
	captionY    = 1250.0

	goldInline  = `{\1c&H00D7FF&}` // #FFD700 in ASS BBGGRR
	whiteInline = `{\1c&HFFFFFF&}`
)

// GenerateASS renders the full subtitle file: Title Block events plus one
// Caption event per word-highlight interval.
func GenerateASS(title, subtitle string, titleSize float64, pages []Page, mainDuration float64) string {
	var b strings.Builder
	b.WriteString("[Script Info]\n")
	b.WriteString("ScriptType: v4.00+\n")
	b.WriteString("PlayResX: 1080\nPlayResY: 1920\n")
	b.WriteString("WrapStyle: 2\nScaledBorderAndShadow: yes\n\n")

	b.WriteString("[V4+ Styles]\n")
	b.WriteString("Format: Name, Fontname, Fontsize, PrimaryColour, SecondaryColour, OutlineColour, BackColour, Bold, Italic, Underline, StrikeOut, ScaleX, ScaleY, Spacing, Angle, BorderStyle, Outline, Shadow, Alignment, MarginL, MarginR, MarginV, Encoding\n")
	// Title: Anton, white, slight shadow. Alignment 8 = top-center.
	fmt.Fprintf(&b, "Style: Title,Anton,%s,&H00FFFFFF,&H00FFFFFF,&H00000000,&HA0000000,0,0,0,0,100,100,%g,0,1,0,2,8,54,54,0,1\n",
		trimFloat(titleSize), TitleLetterSpacing)
	// Subtitle: Montserrat Bold, gold (#FFD700 -> &H0000D7FF).
	b.WriteString("Style: Subtitle,Montserrat,32,&H0000D7FF,&H0000D7FF,&H00000000,&HA0000000,-1,0,0,0,100,100,3,0,1,0,1,8,54,54,0,1\n")
	// Caption: Montserrat Bold on a semi-transparent black box (BorderStyle=3,
	// box colour = BackColour; rgba(0,0,0,0.6) -> alpha 0x66).
	b.WriteString("Style: Caption,Montserrat,42,&H00FFFFFF,&H00FFFFFF,&H66000000,&H66000000,-1,0,0,0,100,100,0,0,3,12,0,8,54,54,0,1\n\n")

	b.WriteString("[Events]\n")
	b.WriteString("Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\n")

	end := assTime(mainDuration)
	// Title Block: title line then subtitle line below it.
	fmt.Fprintf(&b, "Dialogue: 0,0:00:00.00,%s,Title,,0,0,0,,{\\an8\\pos(540,%s)}%s\n",
		end, trimFloat(titleY), sanitize(strings.ToUpper(title)))
	subY := titleY + titleSize*1.05 + subtitleGap
	fmt.Fprintf(&b, "Dialogue: 0,0:00:00.00,%s,Subtitle,,0,0,0,,{\\an8\\pos(540,%s)}%s\n",
		end, trimFloat(subY), sanitize(strings.ToUpper(subtitle)))

	// Karaoke caption events.
	for i, p := range pages {
		dispEnd := p.Words[len(p.Words)-1].End
		if next := i + 1; next < len(pages) && pages[next].Start() < dispEnd {
			dispEnd = pages[next].Start()
		}
		if dispEnd > mainDuration {
			dispEnd = mainDuration
		}
		for _, iv := range pageIntervals(p, dispEnd) {
			fmt.Fprintf(&b, "Dialogue: 0,%s,%s,Caption,,0,0,0,,{\\an5\\pos(540,%s)}%s\n",
				assTime(iv.start), assTime(iv.end), trimFloat(captionY), pageText(p, iv.active))
		}
	}
	return b.String()
}

type interval struct {
	start, end float64
	active     int // index into page words, -1 = none
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
