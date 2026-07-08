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
	// The title starts at TitleStartSize px. If it doesn't fit within
	// TitleMaxWidth it first breaks into two lines at the most balanced word
	// boundary, and only then shrinks; below TitleFloorSize the job is
	// rejected as "title too long".
	TitleStartSize = 88.0
	TitleFloorSize = 48.0
	TitleMaxWidth  = 972.0 // 90% of OutputWidth

	// TitleLetterSpacing is extra px between characters (ASS \fsp).
	TitleLetterSpacing = 2.0
	// TitleY is the distance from the frame top to the title's top edge.
	TitleY = 200.0
	// TitleLineHeight is the per-line vertical advance as a multiple of the
	// font size (used to place line 2 and the subtitle).
	TitleLineHeight = 1.05

	// Subtitle (gold second line) size and its gap below the title.
	SubtitleSize = 32.0
	SubtitleGap  = 18.0
)

// ---------------------------------------------------------------------------
// Karaoke captions
// ---------------------------------------------------------------------------
const (
	CaptionSize = 42.0
	// CaptionY is the vertical center of the caption pill (lower-middle,
	// TikTok-style: ~65% of frame height).
	CaptionY = 1250.0
	// MaxPageDur: words starting within this window of a page's first word
	// share that caption page.
	MaxPageDur = 1.2

	// GoldBGR is the highlight/subtitle color in ASS BBGGRR hex order
	// (#FFD700 gold -> blue=00, green=D7, red=FF).
	GoldBGR = "00D7FF"
	// CaptionBoxAlphaHex is the ASS alpha of the caption pill background
	// (00 = opaque, FF = invisible; 66 ≈ rgba(0,0,0,0.6)).
	CaptionBoxAlphaHex = "66"
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
	TTSVoice          = "en-US-GuyNeural"
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
