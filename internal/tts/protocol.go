package tts

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"html"
	"strings"
	"time"
)

const Voice = "en-US-GuyNeural"

type WordStamp struct {
	Text       string
	Start, End time.Duration
}

func splitTextMessage(msg string) (map[string]string, string) {
	headerPart, body, _ := strings.Cut(msg, "\r\n\r\n")
	headers := map[string]string{}
	for _, line := range strings.Split(headerPart, "\r\n") {
		if k, v, ok := strings.Cut(line, ":"); ok {
			headers[k] = v
		}
	}
	return headers, body
}

// parseBinaryMessage splits an audio frame: 2-byte big-endian header length,
// text headers, then raw payload.
func parseBinaryMessage(msg []byte) (map[string]string, []byte, error) {
	if len(msg) < 2 {
		return nil, nil, fmt.Errorf("tts: binary message too short (%d bytes)", len(msg))
	}
	hlen := int(binary.BigEndian.Uint16(msg[:2]))
	if len(msg) < 2+hlen {
		return nil, nil, fmt.Errorf("tts: header length %d exceeds message", hlen)
	}
	headers, _ := splitTextMessage(string(msg[2 : 2+hlen]))
	return headers, msg[2+hlen:], nil
}

type metadataPayload struct {
	Metadata []struct {
		Type string `json:"Type"`
		Data struct {
			Offset   int64 `json:"Offset"`   // 100ns ticks
			Duration int64 `json:"Duration"` // 100ns ticks
			Text     struct {
				Text string `json:"Text"`
			} `json:"text"`
		} `json:"Data"`
	} `json:"Metadata"`
}

func parseMetadata(body []byte) ([]WordStamp, error) {
	var mp metadataPayload
	if err := json.Unmarshal(body, &mp); err != nil {
		return nil, fmt.Errorf("tts: metadata: %w", err)
	}
	var out []WordStamp
	for _, m := range mp.Metadata {
		if m.Type != "WordBoundary" {
			continue
		}
		start := time.Duration(m.Data.Offset * 100)
		out = append(out, WordStamp{
			Text:  m.Data.Text.Text,
			Start: start,
			End:   start + time.Duration(m.Data.Duration*100),
		})
	}
	return out, nil
}

func buildSSML(script string) string {
	return fmt.Sprintf("<speak version='1.0' xmlns='http://www.w3.org/2001/10/synthesis' xml:lang='en-US'>"+
		"<voice name='%s'><prosody pitch='+0Hz' rate='+0%%' volume='+0%%'>%s</prosody></voice></speak>",
		Voice, html.EscapeString(script))
}
