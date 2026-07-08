// Package captions turns word timestamps into caption pages and generates
// the ASS subtitle file (title block + karaoke captions).
package captions

import (
	"fmt"
	"math"
	"os"
	"strings"
	"unicode/utf8"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"

	"github.com/lascade/motwr/internal/config"
)

func LoadFont(path string) (*sfnt.Font, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("font: %w", err)
	}
	f, err := opentype.Parse(b)
	if err != nil {
		return nil, fmt.Errorf("font %s: %w", path, err)
	}
	return f, nil
}

// TextWidth returns the advance width in pixels of text at sizePx, including
// letterSpacing px between characters (matching ASS \fsp behaviour).
func TextWidth(f *sfnt.Font, text string, sizePx, letterSpacing float64) (float64, error) {
	face, err := opentype.NewFace(f, &opentype.FaceOptions{
		Size: sizePx, DPI: 72, Hinting: font.HintingNone,
	})
	if err != nil {
		return 0, err
	}
	defer face.Close()
	w := float64(font.MeasureString(face, text)) / 64.0
	if n := utf8.RuneCountInString(text); n > 1 {
		w += letterSpacing * float64(n-1)
	}
	return w, nil
}

// TitleLayout is a fitted title: one or two uppercased lines and the font
// size that renders every line within config.TitleMaxWidth.
type TitleLayout struct {
	Lines []string
	Size  float64
}

// LayoutTitle fits a title. It stays a single line at TitleStartSize when it
// fits; otherwise it first breaks into two lines at the most balanced word
// boundary (breaking happens only when necessary), and only then shrinks the
// font to fit the widest line — TitleFloorSize is the limit, below which the
// title is rejected. A single unbreakable word skips straight to shrinking.
func LayoutTitle(f *sfnt.Font, title string) (TitleLayout, error) {
	upper := strings.ToUpper(strings.Join(strings.Fields(title), " "))
	w, err := TextWidth(f, upper, config.TitleStartSize, config.TitleLetterSpacing)
	if err != nil {
		return TitleLayout{}, err
	}
	if w <= config.TitleMaxWidth {
		return TitleLayout{Lines: []string{upper}, Size: config.TitleStartSize}, nil
	}

	words := strings.Fields(upper)
	if len(words) < 2 {
		size, err := FitTitleSize(f, upper, config.TitleMaxWidth,
			config.TitleStartSize, config.TitleFloorSize, config.TitleLetterSpacing)
		if err != nil {
			return TitleLayout{}, err
		}
		return TitleLayout{Lines: []string{upper}, Size: size}, nil
	}

	// Pick the two-line break that minimizes the wider line's width.
	bestWidth := math.Inf(1)
	var bestLines []string
	var widerLine string
	for i := 1; i < len(words); i++ {
		l1 := strings.Join(words[:i], " ")
		l2 := strings.Join(words[i:], " ")
		w1, err := TextWidth(f, l1, config.TitleStartSize, config.TitleLetterSpacing)
		if err != nil {
			return TitleLayout{}, err
		}
		w2, err := TextWidth(f, l2, config.TitleStartSize, config.TitleLetterSpacing)
		if err != nil {
			return TitleLayout{}, err
		}
		wide, wideLine := w1, l1
		if w2 > w1 {
			wide, wideLine = w2, l2
		}
		if wide < bestWidth {
			bestWidth, bestLines, widerLine = wide, []string{l1, l2}, wideLine
		}
	}

	size, err := FitTitleSize(f, widerLine, config.TitleMaxWidth,
		config.TitleStartSize, config.TitleFloorSize, config.TitleLetterSpacing)
	if err != nil {
		return TitleLayout{}, err
	}
	return TitleLayout{Lines: bestLines, Size: size}, nil
}

// FitTitleSize shrinks startSize until text fits maxWidth. Advance widths
// scale roughly linearly with size, so ratio steps converge fast, but face
// metrics are quantized — the descent loop below absorbs the residual.
func FitTitleSize(f *sfnt.Font, text string, maxWidth, startSize, floorSize, letterSpacing float64) (float64, error) {
	size := startSize
	for range 5 {
		w, err := TextWidth(f, text, size, letterSpacing)
		if err != nil {
			return 0, err
		}
		if w <= maxWidth {
			return size, nil
		}
		if next := size * maxWidth / w; next < size-0.25 {
			size = next
		} else {
			size -= 0.25
		}
		if size < floorSize {
			break
		}
	}
	for size >= floorSize {
		w, err := TextWidth(f, text, size, letterSpacing)
		if err != nil {
			return 0, err
		}
		if w <= maxWidth {
			return size, nil
		}
		size -= 0.25
	}
	return 0, fmt.Errorf("title too long: no fitting size at or above the %.0fpx floor", floorSize)
}
