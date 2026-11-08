package tts

import (
	"regexp"
	"testing"
	"time"
)

func TestGenerateSecMSGECFormat(t *testing.T) {
	tok := GenerateSecMSGEC(time.Date(2026, 7, 8, 12, 0, 0, 0, time.UTC))
	if !regexp.MustCompile(`^[0-9A-F]{16}-[0-9A-F]{64}$`).MatchString(tok) {
		t.Fatalf("token %q is not a v2 Sec-MS-GEC token", tok)
	}
}

func TestGenerateSecMSGECMatchesCapturedToken(t *testing.T) {
	// Captured from Edge Read Aloud for the 2026-10-13 10:40 UTC window.
	want := "492D7694A183A45E-56E821F95D4579C5B7C7E33DC7303FF379EC36D510213555BBBB9EB17EF15876"
	got := GenerateSecMSGEC(time.Date(2026, 10, 13, 10, 40, 0, 0, time.UTC))
	if got != want {
		t.Fatalf("GenerateSecMSGEC() = %s, want %s", got, want)
	}
}

func TestGenerateSecMSGECStableWithin5MinWindow(t *testing.T) {
	base := time.Date(2026, 7, 8, 12, 0, 1, 0, time.UTC)
	if GenerateSecMSGEC(base) != GenerateSecMSGEC(base.Add(4*time.Minute)) {
		t.Error("token must be stable inside one 5-minute window")
	}
	if GenerateSecMSGEC(base) == GenerateSecMSGEC(base.Add(6*time.Minute)) {
		t.Error("token must change across 5-minute windows")
	}
}
