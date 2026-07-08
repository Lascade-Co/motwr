// Package tts implements the edge-tts websocket protocol natively: one
// connection yields both the voiceover MP3 and per-word WordBoundary
// timestamps (ADR-0001).
package tts

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

const trustedClientToken = "6A5AA1D4EAFF4E9FB37E23D68491D6F4"

// GenerateSecMSGEC computes the Sec-MS-GEC DRM header: SHA-256 of the
// current Windows file time (100ns ticks since 1601-01-01) rounded down to a
// 5-minute boundary, concatenated with the trusted client token.
func GenerateSecMSGEC(now time.Time) string {
	const windowsEpochDiff = 11644473600 // seconds 1601-01-01 -> 1970-01-01
	sec := now.UTC().Unix() + windowsEpochDiff
	sec -= sec % 300
	ticks := sec * 10_000_000
	sum := sha256.Sum256([]byte(fmt.Sprintf("%d%s", ticks, trustedClientToken)))
	return strings.ToUpper(hex.EncodeToString(sum[:]))
}
