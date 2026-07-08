# Word timestamps come from TTS, not transcription

The reference render server generated the voiceover, then transcribed it with
whisper.cpp to recover caption timing. We instead implement the edge-tts
websocket protocol natively in Go: one connection streams both the voiceover
audio and per-word `WordBoundary` events, so caption timing is exact and free.
This removes whisper.cpp, its model download, and the python/edge-tts CLI
dependency — the binary's only external requirement is ffmpeg.

## Consequences

- Captions can never mis-transcribe the script (timing and text come from the
  same source), but the pipeline is coupled to edge-tts: swapping in a TTS
  engine that doesn't emit word timing (or ElevenLabs without its
  with-timestamps endpoint) would force a transcription step back in.
- The voice is hardcoded (`en-US-GuyNeural`) by explicit decision — no flag.
