# Audio Clip to Video (ac2v) — Work Plan (Current)

## Goal
Build a **local-first CLI tool in Go (Windows + NVIDIA)** that converts a **10–30s
audio clip** into **three videos** (1:1, 16:9, 9:16) by:
- transcribing audio (local `whisper.cpp`, model `medium.en`)
- generating **chunked SRT subtitles** (bottom-center)
- choosing images from a **local folder (10–15 images)** using a **local LLM**
  (Ollama) driven by **image captions/tags**
- rendering with **ffmpeg** using **blurred background + centered foreground**
- using **hard cuts** and **2.5s minimum shot duration**
- caching intermediates for fast reruns

Non-goals (MVP):
- word-level alignment (segment timing is acceptable)
- transitions (no fades)
- safe-area subtitle avoidance
- network calls (beyond one-time model downloads via user tooling)

---

## Runtime Requirements (User-installed)
1. `ffmpeg` available on PATH
2. `whisper.cpp` binary available on PATH (preferably CUDA-enabled)
3. `ollama` installed and running locally
4. Ollama models pulled once:
   - Vision captioning: `llava:7b`
   - Text selection: `llama3.1:8b-instruct`

Suggested one-time setup:
```bash
ollama pull llava:7b
ollama pull llama3.1:8b-instruct
```

---

## Defaults (Locked In)
- Clip length: 10–30 seconds
- Images: local folder, typically 10–15
- Transcription:
  - English only
  - Whisper model: `medium.en`
  - Segment timing acceptable
- Video:
  - FPS: 30
  - Aspects + resolutions:
    - 1:1 = 1080x1080
    - 16:9 = 1920x1080
    - 9:16 = 1080x1920
- Visuals:
  - min shot duration: 2.5s
  - hard cuts
  - blur background (no crop), centered-fit foreground
- Subtitles:
  - chunked SRT
  - bottom-center

---

## CLI Design (MVP)
Primary command:
```bash
ac2v run --audio <file> --images <dir> --out <dir>
```

Recommended optional flags:
- `--aspects 1x1,16x9,9x16`
- `--min-shot 2.5`
- `--max-words 5`
- `--blur 20`
- `--workdir <dir>`
- `--whisper-model medium.en`
- `--ollama-host http://localhost:11434`
- `--vision-model llava:7b`
- `--select-model llama3.1:8b-instruct`

Subcommands (for iteration/debug):
- `ac2v transcribe ...`
- `ac2v caption-images ...`
- `ac2v plan ...`
- `ac2v subs ...`
- `ac2v render ...`

---

## Data & Caching Layout
Each run uses a deterministic work directory keyed by:
- audio hash
- image file list + hashes
- settings hash

Example:
```
.work/<hash>/
  project.json
  audio.wav
  transcript.json
  subtitles.srt
  images/
    index.json
    captions.json
  text/
    windows.json
  timeline.json
  render/
    concat.txt
```

Caching rules:
- If an image hash hasn’t changed and model name hasn’t changed, reuse its
  caption result.
- If audio hash/settings haven’t changed, reuse transcript/windows/subtitles.
- Always allow `--force` to recompute a stage.

---

## Pipeline (End-to-End)

### 0) Preflight
- verify `ffmpeg` is callable
- verify `whisper.cpp` is callable
- verify Ollama is reachable (`GET /api/tags` or a small test request)
- validate inputs: audio exists, image folder has at least 1 image

Outputs:
- `project.json` (inputs, settings, versions)

---

### 1) Audio Normalize (ffmpeg)
Convert input audio to a whisper-friendly format:
- mono, 16kHz PCM WAV

Output:
- `audio.wav`

Example command:
```bash
ffmpeg -y -i "input.mp3" -ac 1 -ar 16000 -c:a pcm_s16le ".work/.../audio.wav"
```

---

### 2) Transcribe (whisper.cpp)
Run whisper.cpp using `medium.en`.

Output:
- `transcript.json` normalized to:
  - `language`
  - `durationSec`
  - `segments[]: {start, end, text}`

Notes:
- Segment-level timestamps are acceptable.
- If whisper.cpp can emit SRT/JSON directly, parse and normalize.

---

### 3) Build Transcript Windows (2.5s cadence)
Create fixed windows across `[0, duration]`:
- window size = 2.5s

For each window:
- collect overlapping segment text
- store `{start, end, text}`

Output:
- `text/windows.json`

---

### 4) Caption Images (Ollama vision model)
For each image (10–15):
- send image to Ollama vision model (`llava:7b`)
- request JSON-only output:
  - caption (1 sentence)
  - tags (8–15)
  - style (0–5)
  - notes (optional)

Output:
- `images/captions.json`

Reliability:
- Use `temperature=0`
- Use Ollama JSON mode if available (`format: "json"`)
- If JSON parse fails: retry once with a stricter “ONLY JSON” prompt.

---

### 5) Plan Timeline (Ollama text model)
For each 2.5s window:
- provide window text
- provide the full image catalog (IDs + caption + tags)
- provide `previous_image_id` to discourage repeats
- ask the LLM to output JSON:
  - `chosen_image_id`
  - `backup_image_id`
  - `confidence` (0–1)
  - `reason` (short)

Apply post-rules in Go:
- if the model repeats the same image and there are alternatives, switch to
  `backup_image_id`
- if confidence is very low, optionally pick a default/fallback image

Output:
- `timeline.json`:
  - `[{start, end, image_id, confidence, reason}]`

---

### 6) Generate Chunked SRT
Using segment timing:
- split each segment into words
- approximate per-word timing within the segment
- group into chunks (default `maxWords=5`)
- write SRT cues, bottom-center (position handled at render stage)

Output:
- `subtitles.srt`

---

### 7) Render (ffmpeg) — 3 aspects
For each aspect/resolution:
- build an ffconcat file from timeline (still images + durations)
- render with:
  - background: scale-to-fill + crop + blur
  - foreground: scale-to-fit (no crop)
  - overlay foreground centered
  - burn subtitles from SRT
  - mux original audio
  - set fps = 30
  - hard cuts (no xfade)

Outputs:
- `out/<name>_1x1.mp4`
- `out/<name>_16x9.mp4`
- `out/<name>_9x16.mp4`

Render artifact:
- `render/concat.txt`

---

## Rendering Strategy (Blur Background + Center Fit)
Conceptual filter chain per aspect (W x H):
1. split input video into two streams
2. background:
   - scale to cover
   - crop to W:H
   - blur
3. foreground:
   - scale to contain (no crop)
4. overlay foreground on background
5. burn subtitles
6. format yuv420p

Note:
- Windows path quoting can be tricky; Go should pass ffmpeg args as an array and
  avoid shell quoting.

---

## Implementation Phases

### Phase 1 — “Hello world” end-to-end (single aspect)
- Go CLI skeleton (`run`)
- ffmpeg normalize audio
- whisper.cpp transcription -> `transcript.json`
- generate SRT
- simple placeholder timeline (cycle images)
- render 1:1 output with blur background and subtitles

Deliverable:
- `ac2v run ...` produces one playable MP4.

### Phase 2 — Add Ollama captioning + LLM selection
- `caption-images` step using `llava:7b`
- `plan` step using `llama3.1:8b-instruct`
- caching + `--force`
- timeline smoothing (avoid immediate repeats)

Deliverable:
- image choices respond to narration content.

### Phase 3 — Multi-aspect outputs
- render 1:1, 16:9, 9:16 in one run
- validate subtitle readability across aspects

Deliverable:
- 3 MP4s per run.

### Phase 4 — Debuggability + control
- `--preview` (prints timeline + chosen images)
- `--exclude <pattern>`
- `--seed` (tie-breaking; keep selection stable)
- better error messages (missing deps, Ollama not running, etc.)

---

## Risks / Notes
- LLM output parsing: enforce JSON-only + retry strategy.
- Caption quality varies; keep captions/tag prompts tight and consistent.
- Transcription timing: segment-based chunk timing is approximate but acceptable
  for MVP.
- ffmpeg concat demuxer requires the last file repeated in the concat list.

---

## Open Items (Future Enhancements)
- word-level alignment (whisperX or forced alignment)
- transitions (simple fades)
- safe area logic for subtitles
- optional CLIP embeddings as a fallback or hybrid ranker
- user overrides: pin image to time window, reorder, disallow repeats, etc.
```