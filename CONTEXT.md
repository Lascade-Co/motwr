# motwr — Route Video Rendering

A standalone Go CLI that turns a TravelAnimator map-export video plus a job
payload into a finished 1080×1920 branded short with voiceover, captions, and
overlays. Replaces the Node/Remotion render server.

## Language

**Base Video**:
The user-supplied 9:16 map-animation mp4 that forms the visual background.
_Avoid_: input video, map video, source video

**Job**:
The JSON payload (title, subtitle, script, vehicle) describing one render.
_Avoid_: payload, request

**Script**:
The narration text inside a Job, converted to speech by TTS.
_Avoid_: captions script, text

**Voiceover**:
The TTS-generated narration audio; its duration defines the Main Segment length.
_Avoid_: audio, narration file

**Word Timestamps**:
Per-word start/duration timing emitted by edge-tts WordBoundary events.
_Avoid_: transcription, captions.json

**Speed Ramp**:
A uniform retime of the Base Video (single constant setpts factor) so it spans
exactly the Voiceover duration. NOT a non-linear speed curve.
_Avoid_: speed curve, time remap

**Caption Page**:
A ≤1.2 s group of consecutive words shown together — Poppins Bold Italic,
white, outlined, no background box, lower-middle. Long pages wrap to two
lines. The word currently being spoken is highlighted gold (#FFD700), the rest
white (dynamic word-by-word highlight synced to the Voiceover).
_Avoid_: subtitle line, caption block

**Title Block**:
The heading overlay — Job title (Anton, white, uppercased, 1-2 lines; any
trailing "(...)" qualifier is dropped) over subtitle (Montserrat Bold, gold,
uppercased) — pinned top-center for the whole Main Segment. A title too wide
at full size first breaks into two lines at the most balanced *top-heavy*
word boundary — the first line is the longer one (only when necessary); only
then does the font shrink to fit the widest line (floor 48 px; below that the
Job is rejected as "title too long").

**Bird Appearance**:
One scheduled overlay of a randomly-picked bird webm (alpha) plus its
bird-sfx instance. Appearances never overlap: each starts 10 s after the
previous one ends (gap-based), and every clip plays its natural length.

**Vehicle**:
Enum {car, boat, plane, train}; selects the background ambience track
`background/{vehicle}.mp3`.

**Main Segment**:
The rendered portion from t=0 to the Voiceover end — everything except the Outro.

**Outro**:
The pre-rendered brand clip (`outro.mp4`) appended unmodified after the Main
Segment.

## Relationships

- A **Job** produces exactly one output video: **Main Segment** + **Outro**
- The **Voiceover** duration determines the **Speed Ramp** factor and the
  **Main Segment** length
- **Word Timestamps** are grouped into **Caption Pages**
- Each **Bird Appearance** pairs one bird clip with one bird-sfx instance

## Example dialogue

> **Dev:** "Does the **Speed Ramp** ease in and out?"
> **Domain expert:** "No — it's a uniform retime; the **Base Video** is
> stretched or compressed by one constant factor to match the **Voiceover**."
> **Dev:** "And if the **Script** is long, the **Outro** gets pushed later?"
> **Domain expert:** "Yes — the **Main Segment** is exactly as long as the
> **Voiceover**; the **Outro** always follows it untouched."

## Flagged ambiguities

- "speed ramp" was used to mean a non-linear speed curve in general editing
  parlance — resolved: in this context it is strictly a uniform retime.
