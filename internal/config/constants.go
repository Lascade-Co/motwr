// Package config is the single place to adjust motwr's tunable values:
// layout, colors, audio levels, timing, and encoding parameters. Everything
// is a compile-time constant — rebuild after changing.
//
// Positions and sizes are in output pixels (the ASS PlayRes equals the
// output resolution, so 1 unit = 1 px).
package config

import "time"

// ---------------------------------------------------------------------------
// Output format
// ---------------------------------------------------------------------------
const (
	// Final video resolution (9:16 portrait) and constant frame rate.
	OutputWidth  = 1080
	OutputHeight = 1920
	OutputFPS    = 30

	// AspectTolerance is how far the base video's width/height ratio may
	// deviate from 9:16 before the input is rejected.
	AspectTolerance = 0.005

	// VideoCRF is x264 quality: lower = better and larger (18 ≈ visually
	// lossless). VideoPreset trades encode speed for compression.
	VideoCRF    = 18
	VideoPreset = "medium"

	// Audio encode parameters — chosen to match assets/outro.mp4 so the
	// outro can be appended without re-encoding (ADR-0002).
	AudioBitrate    = "192k"
	AudioSampleRate = 44100
	AudioChannels   = 2
)

// ---------------------------------------------------------------------------
// Title block (top-center heading)
// ---------------------------------------------------------------------------
const (
	// TitleFont is the ASS family name of the title (heading) face. Must equal
	// the font's internal family name (libass matches by family, not filename).
	// Realist Clostan Black — the brand display face for headings.
	TitleFont = "Realist Clostan"

	// The title starts at TitleStartSize px. If it doesn't fit within
	// TitleMaxWidth it first breaks into two lines (top-heavy), and only then
	// shrinks; below TitleFloorSize the job is rejected as "title too long".
	// 170 fills most of the frame width for a typical 2-3 word title, like
	// the reference; longer titles wrap and, only if needed, shrink.
	TitleStartSize = 170.0
	TitleFloorSize = 48.0
	// TitleMaxWidth is the fitting budget in font-metric space (not rendered
	// pixels). LayoutTitle measures advance widths with golang.org/x/image,
	// whose metric runs at a fixed ratio to the libass render; the budget must
	// correspond to the 972 px usable width left by TitleMargin times that
	// ratio, so a big single-line title fills the frame instead of wrapping.
	// If the budget is too high libass hard-wraps each line and the whole
	// title block collapses. Calibrated for Realist Clostan Black: the Go
	// metric runs ~1.049x the libass render, so 972 * 1.049 ≈ 1019.
	TitleMaxWidth = 1019.0
	// TitleMargin is the left/right ASS margin (rendered px) for the title and
	// subtitle. 1080 - 2*54 = 972 px usable.
	TitleMargin = 54.0

	// TitleLetterSpacing is extra px between characters (ASS \fsp).
	TitleLetterSpacing = 2.0
	// TitleY is the distance from the frame top to the title's top edge.
	TitleY = 200.0
	// TitleLineHeight is the per-line vertical advance as a multiple of the
	// font size; used to place line 2 and the subtitle. Tight (an all-caps
	// display face has no descenders) so a two-line heading doesn't gap open.
	TitleLineHeight = 0.75
	// TitleOutline/TitleShadow are the ASS border and drop-shadow widths (px).
	TitleOutline = 0.0
	TitleShadow  = 2.0

	// SubtitleFont is the ASS family name of the subtitle (sub-heading) face.
	// Bebas Neue — a tall condensed all-caps face; rendered uppercase in gold.
	SubtitleFont = "Bebas Neue"

	// Subtitle (gold line) size, its gap below the title, and outline width.
	SubtitleSize    = 56.0
	SubtitleGap     = 20.0
	SubtitleOutline = 2.0
)

// ---------------------------------------------------------------------------
// Captions
// ---------------------------------------------------------------------------
const (
	// CaptionFont is the ASS family name of the caption face. Quicksand — a
	// rounded sans, rendered Bold (the Caption style sets the ASS bold flag,
	// so libass selects Quicksand-Bold.ttf). White with a gold word-by-word
	// highlight, a simple black outline, no box.
	CaptionFont = "Quicksand"

	// CaptionSize is +10% over the previous 66 px for a chunkier caption.
	CaptionSize = 72.6
	// CaptionY is the vertical center of the caption block (lower-middle,
	// TikTok-style: ~63% of frame height).
	CaptionY = 1210.0
	// CaptionMargin is the left/right ASS margin (px). 1080 - 2*300 = 480 px
	// usable, so a multi-word page wraps to two lines like the reference.
	CaptionMargin = 300.0
	// CaptionOutline/CaptionShadow are the ASS border and drop-shadow widths.
	// Shadow is 0: captions use a simple black outline only, no drop shadow.
	CaptionOutline = 5.0
	CaptionShadow  = 0.0
	// CaptionLetterSpacing is extra px between characters (ASS \fsp).
	CaptionLetterSpacing = 1.0

	// MaxPageDur: words starting within this window of a page's first word
	// share that caption page.
	MaxPageDur = 1.2

	// GoldBGR is the subtitle color in ASS BBGGRR hex order
	// (#FFD700 gold -> blue=00, green=D7, red=FF).
	GoldBGR = "00D7FF"
)

// ---------------------------------------------------------------------------
// Radial spotlight
// ---------------------------------------------------------------------------
//
// A radial (elliptical) brightness falloff applied to the base map footage:
// full brightness in the center — where the vehicle travels — fading to a
// dim floor toward the frame edges, so the vehicle and its surroundings read
// as highlighted. Applied before the bird/logo/caption overlays, so only the
// map is darkened. Radii are expressed in normalized frame units where 1.0
// reaches the mid-edge (left/right or top/bottom) and ~1.41 reaches a corner.
const (
	// SpotlightEnabled toggles the whole effect. Set false to render flat.
	SpotlightEnabled = true

	// SpotlightCenterX/Y is the bright center as a fraction of frame
	// width/height. 0.5,0.5 is the exact frame center (where the vehicle sits).
	SpotlightCenterX = 0.5
	SpotlightCenterY = 0.5

	// SpotlightInnerRadius: everything within this normalized radius keeps full
	// brightness. SpotlightOuterRadius: brightness reaches its floor here and
	// beyond. Between the two the falloff is a smooth (smoothstep) ramp.
	SpotlightInnerRadius = 0.40
	SpotlightOuterRadius = 1.10

	// SpotlightEdgeBrightness is the brightness multiplier at/beyond the outer
	// radius (0..1). 0.35 leaves the darkened edges at 35% of their brightness.
	SpotlightEdgeBrightness = 0.35
)

// ---------------------------------------------------------------------------
// Audio mix
// ---------------------------------------------------------------------------
const (
	VoiceoverVolume   = 1.5 // narration boost
	BackgroundVolume  = 0.3 // vehicle ambience bed
	BackgroundFadeOut = 1.5 // seconds, at the end of the main segment
	BirdSFXVolume     = 0.4
	BirdSFXFadeOut    = 0.3 // seconds, at the end of each appearance
)

// ---------------------------------------------------------------------------
// Bird overlays
// ---------------------------------------------------------------------------
const (
	// BirdGap is the pause between one bird leaving and the next appearing.
	BirdGap = 10.0
	// BirdMinAppearance: appearances shorter than this (at the video's end)
	// are skipped.
	BirdMinAppearance = 0.5
	// BirdClipCount: clips are assets/bird/0.webm .. (BirdClipCount-1).webm.
	BirdClipCount = 4
)

// ---------------------------------------------------------------------------
// Logo (top-right corner)
// ---------------------------------------------------------------------------
const (
	LogoWidth   = 218 // px, height scales to preserve aspect
	LogoPadding = 40  // px from the top and right edges
)

// ---------------------------------------------------------------------------
// Text-to-speech (edge-tts)
// ---------------------------------------------------------------------------
const (
	TTSVoice          = "en-US-AndrewNeural"
	TTSMaxScriptBytes = 32 << 10 // one websocket turn; ample for narration
	TTSAttempts       = 3        // total tries before giving up
)

// ---------------------------------------------------------------------------
// Miscellaneous
// ---------------------------------------------------------------------------
const (
	// JobFetchTimeout bounds downloading the job JSON from a URL.
	JobFetchTimeout = 30 * time.Second
	// ConcatDurationTolerance: the final video may be at most this many
	// seconds shorter than main+outro before the concat is declared broken.
	ConcatDurationTolerance = 0.5
)
