# Pure-ffmpeg single-pass render replaces Remotion

The reference implementation rendered overlays (title, karaoke captions, logo,
birds) with Remotion, which drags in Node, a webpack bundle, and headless
Chromium. We render everything in one ffmpeg pass instead: title block and
karaoke captions as a generated ASS file burned in via libass, bird/logo
compositing and audio mixing in a single filter graph, and the outro appended
by stream-copy concat (the main segment is encoded to match outro.mp4's
parameters exactly so no re-encode is needed).

## Considered options

- **Remotion (reference)** — rejected: Chromium + Node runtime contradicts the
  standalone-binary goal, and renders are far slower than one ffmpeg encode.
- **Staged ffmpeg passes with intermediates** — rejected: 3–4× encode time and
  generational quality loss; kept only as a debugging technique.
- **Go-native frame compositing** — rejected: reimplements libass/ffmpeg badly.

## Consequences

- Styling lives in ASS markup and filter-graph strings, not React components —
  layout changes mean regenerating ASS/filter expressions, not JSX edits.
- Bird webms must be decoded with `-vcodec libvpx-vp9`; ffmpeg's native VP9
  decoder silently drops the alpha channel.
- The outro is re-encoded (~3 s clip) with the pipeline's own encoder
  settings before the stream-copy concat: "matching parameters by
  construction" proved false in practice (the shipped outro.mp4 is H.264
  Main profile at 1/30 track timescale vs our High at 1/15360), and
  copy-concatenating mismatched streams silently collapses the outro video
  into milliseconds while its audio plays. Concat results are verified by
  per-video-stream duration, with a full re-encoding concat as fallback.
