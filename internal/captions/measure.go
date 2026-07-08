// Package captions turns word timestamps into caption pages and generates
// the ASS subtitle file (title block + karaoke captions).
package captions

import (
	"fmt"
	"os"
	"unicode/utf8"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
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
