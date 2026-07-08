package tts

import (
	"strings"
	"testing"
	"time"
)

func TestSplitTextMessage(t *testing.T) {
	msg := "X-RequestId:abc\r\nPath:audio.metadata\r\n\r\n{\"Metadata\":[]}"
	h, body := splitTextMessage(msg)
	if h["Path"] != "audio.metadata" {
		t.Errorf("Path=%q", h["Path"])
	}
	if body != `{"Metadata":[]}` {
		t.Errorf("body=%q", body)
	}
}

func TestParseBinaryMessage(t *testing.T) {
	header := "Path:audio\r\nContent-Type:audio/mpeg"
	payload := []byte{0xFF, 0xF3, 0x01, 0x02}
	msg := append([]byte{0, byte(len(header))}, []byte(header)...)
	msg = append(msg, payload...)
	h, got, err := parseBinaryMessage(msg)
	if err != nil {
		t.Fatal(err)
	}
	if h["Path"] != "audio" {
		t.Errorf("Path=%q", h["Path"])
	}
	if string(got) != string(payload) {
		t.Errorf("payload=%v", got)
	}
}

func TestParseBinaryMessageTooShort(t *testing.T) {
	if _, _, err := parseBinaryMessage([]byte{0}); err == nil {
		t.Error("expected error")
	}
}

func TestParseMetadataWordBoundary(t *testing.T) {
	body := `{"Metadata":[
	  {"Type":"WordBoundary","Data":{"Offset":1000000,"Duration":4000000,"text":{"Text":"Our"}}},
	  {"Type":"SentenceBoundary","Data":{"Offset":0,"Duration":0,"text":{"Text":"x"}}},
	  {"Type":"WordBoundary","Data":{"Offset":5500000,"Duration":3000000,"text":{"Text":"journey"}}}]}`
	stamps, err := parseMetadata([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	if len(stamps) != 2 {
		t.Fatalf("got %d stamps, want 2 (SentenceBoundary skipped)", len(stamps))
	}
	if stamps[0].Text != "Our" || stamps[0].Start != 100*time.Millisecond ||
		stamps[0].End != 500*time.Millisecond {
		t.Errorf("stamp[0] = %+v", stamps[0])
	}
	if stamps[1].Text != "journey" || stamps[1].Start != 550*time.Millisecond {
		t.Errorf("stamp[1] = %+v", stamps[1])
	}
}

func TestSSMLEscapesScript(t *testing.T) {
	s := buildSSML("Fish & chips <fast>")
	if want := "Fish &amp; chips &lt;fast&gt;"; !strings.Contains(s, want) {
		t.Errorf("ssml %q missing %q", s, want)
	}
	if !strings.Contains(s, "en-US-GuyNeural") {
		t.Error("ssml missing hardcoded voice")
	}
}
