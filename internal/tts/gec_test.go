package tts

import (
	"regexp"
	"testing"
	"time"
)

func TestGenerateSecMSGECFormat(t *testing.T) {
	tok := GenerateSecMSGEC(time.Date(2026, 7, 8, 12, 0, 0, 0, time.UTC))
	if !regexp.MustCompile(`^[0-9A-F]{64}$`).MatchString(tok) {
		t.Fatalf("token %q is not 64 uppercase hex chars", tok)
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
