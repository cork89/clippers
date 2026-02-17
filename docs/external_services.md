# External Services and Applications

This project depends on several external services and command-line applications. Below is a concise list of what they are and how the project uses them.

## Runtime Requirements

- `ffmpeg`
  - Used to normalize audio input and to render final videos (including aspect-ratio outputs and effects).
- `ffprobe`
  - Used to inspect audio/video metadata as part of the pipeline.
- `whisper-ctranslate2` (Whisper transcription)
  - Used to transcribe audio into text and timing data for subtitles and segmenting.
- `SubtitleEdit`
  - Used to convert subtitle files from SRT to ASS for rendering.
- `ollama`
  - Local LLM service used for vision captioning (image tags/captions) and text-based image selection.

## Optional Runtime Service

- `OpenRouter` (LLM API)
  - Alternative LLM provider to Ollama. Used for vision captioning and text-based image selection when configured.
  - Requires `OPENROUTER_API_KEY`.

## Frontend Runtime CDN

- `HTMX` (loaded from unpkg CDN)
  - Used in the web UI for dynamic HTML updates, server-driven interactions, and websocket extensions.

## Development Tools

- `air`
  - Live-reload development server; rebuilds the binary and reruns `templ generate` on changes.
- `templ` (CLI)
  - Generates `*_templ.go` files from `.templ` templates for the web UI.
