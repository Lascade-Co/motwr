package tts

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"sort"
	"time"

	"github.com/coder/websocket"

	"github.com/lascade/motwr/internal/config"
)

const (
	wssBase    = "wss://speech.platform.bing.com/consumer/speech/synthesize/readaloud/edge/v1"
	gecVersion = "1-143.0.3650.75"
	userAgent  = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/143.0.0.0 Safari/537.36 Edg/143.0.0.0"
	origin     = "chrome-extension://jdiccldimpdaibmpdkjnbmckianbfold"
)

// Synthesize generates the voiceover MP3 at outPath and returns word
// timestamps. Retries transient failures 3 times.
func Synthesize(ctx context.Context, script, outPath string) ([]WordStamp, error) {
	if len(script) > config.TTSMaxScriptBytes {
		return nil, fmt.Errorf("tts: script too long (%d bytes, max %d)", len(script), config.TTSMaxScriptBytes)
	}
	var lastErr error
	for attempt := range config.TTSAttempts {
		stamps, err := synthesizeOnce(ctx, script, outPath)
		if err == nil {
			return stamps, nil
		}
		lastErr = err
		if attempt == config.TTSAttempts-1 {
			break
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Duration(attempt+1) * 2 * time.Second):
		}
	}
	return nil, fmt.Errorf("tts: all attempts failed: %w", lastErr)
}

func synthesizeOnce(ctx context.Context, script, outPath string) ([]WordStamp, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	url := fmt.Sprintf("%s?TrustedClientToken=%s&Sec-MS-GEC=%s&Sec-MS-GEC-Version=%s&ConnectionId=%s",
		wssBase, trustedClientToken, GenerateSecMSGEC(time.Now()), gecVersion, randomHex16())
	hdr := http.Header{}
	hdr.Set("Origin", origin)
	hdr.Set("User-Agent", userAgent)
	hdr.Set("Pragma", "no-cache")
	hdr.Set("Cache-Control", "no-cache")

	conn, _, err := websocket.Dial(ctx, url, &websocket.DialOptions{HTTPHeader: hdr})
	if err != nil {
		return nil, fmt.Errorf("tts: dial: %w", err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "")
	conn.SetReadLimit(1 << 22)

	ts := time.Now().UTC().Format("Mon Jan 02 2006 15:04:05 GMT+0000 (Coordinated Universal Time)")
	cfg := "X-Timestamp:" + ts + "\r\nContent-Type:application/json; charset=utf-8\r\nPath:speech.config\r\n\r\n" +
		`{"context":{"synthesis":{"audio":{"metadataoptions":{"sentenceBoundaryEnabled":"false","wordBoundaryEnabled":"true"},"outputFormat":"audio-24khz-48kbitrate-mono-mp3"}}}}`
	if err := conn.Write(ctx, websocket.MessageText, []byte(cfg)); err != nil {
		return nil, fmt.Errorf("tts: send config: %w", err)
	}
	ssmlMsg := "X-RequestId:" + randomHex16() + "\r\nContent-Type:application/ssml+xml\r\n" +
		"X-Timestamp:" + ts + "Z\r\nPath:ssml\r\n\r\n" + buildSSML(script)
	if err := conn.Write(ctx, websocket.MessageText, []byte(ssmlMsg)); err != nil {
		return nil, fmt.Errorf("tts: send ssml: %w", err)
	}

	var audio []byte
	var stamps []WordStamp
	for {
		typ, data, err := conn.Read(ctx)
		if err != nil {
			return nil, fmt.Errorf("tts: read: %w", err)
		}
		switch typ {
		case websocket.MessageText:
			headers, body := splitTextMessage(string(data))
			switch headers["Path"] {
			case "audio.metadata":
				ws, err := parseMetadata([]byte(body))
				if err != nil {
					return nil, err
				}
				stamps = append(stamps, ws...)
			case "turn.end":
				if len(audio) == 0 {
					return nil, fmt.Errorf("tts: no audio received")
				}
				if len(stamps) == 0 {
					return nil, fmt.Errorf("tts: no word boundaries received")
				}
				if err := os.WriteFile(outPath, audio, 0o644); err != nil {
					return nil, fmt.Errorf("tts: write %s: %w", outPath, err)
				}
				sort.Slice(stamps, func(i, j int) bool { return stamps[i].Start < stamps[j].Start })
				return stamps, nil
			}
		case websocket.MessageBinary:
			headers, payload, err := parseBinaryMessage(data)
			if err != nil {
				return nil, err
			}
			if headers["Path"] == "audio" {
				audio = append(audio, payload...)
			}
		}
	}
}

func randomHex16() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}
