# Clippers – Audio Clip to Video  
**Local-first, AI-driven, multi-format video generator**

Turn a 10-30 s audio clip and a small folder of images into a subtitled video

---

## What It Does

1. **Transcribes** the audio locally (Whisper.cpp)  
2. **Captions** every image with a vision LLM (Ollama `llava`)  
3. **Matches** images to spoken content with a text LLM (Ollama `gemma3`)  
4. **Renders** three MP4s with:
   - Hard-cut edits (≥ 5 s per shot)  
   - Blurred background + centred foreground  
   - Bottom-center chunked subtitles  
   - 30 fps, H.264/AAC, yuv420p

All intermediate files are cached; re-runs are instant if nothing changed.

---

## Install (Windows + NVIDIA)

1. **Binaries** – add these to `PATH`:
   - `ffmpeg` (including `ffprobe`)  
   - `whisper.cpp` binary (CUDA build recommended)  

2. **Ollama** – install once:
   ```bash
   # Windows (PowerShell)
   winget install Ollama.Ollama
   ollama serve          # keep this terminal open
   ```

3. **Pull models** (one-time):
   ```bash
   ollama pull llava:7b
   ollama pull llama3.1:8b-instruct
   ```

4. **clippers** – grab the latest release or build:
   ```bash
   git clone https://github.com/cork89/clippers
   cd clippers
   go build -o clippers.exe ./cmd/clippers
   ```

---

## 30-Second Start

```bash
# drop a short audio file and 10-15 images into a folder
clippers.exe run --audio clip.mp3 --images ./imgs --out ./output
```

After the first run you’ll find:

```
output/
├── clip_1x1.mp4    # 1080×1080  – Instagram / Facebook
├── clip_16x9.mp4   # 1920×1080 – YouTube / LinkedIn
└── clip_9x16.mp4   # 1080×1920 – TikTok / Reels
```

---

## CLI Reference

### Main command
```
clippers run -a <audio> -i <images> -o <output> [flags]
```

| flag | default | meaning |
|------|---------|---------|
| `--aspects` | `1x1,16x9,9x16` | comma-separated list (`1x1` `16x9` `9x16`) |
| `--min-shot` | `4` | minimum shot length (seconds) |
| `--max-words` | `5` | words per subtitle cue |
| `--blur` | `20` | background blur strength |
| `--font-size` | `40` | base subtitle font size |
| `--whisper-model` | `medium.en` | whisper model |
| `--ollama-host` | `http://localhost:11434` | Ollama API |
| `--vision-model` | `llava:7b` | captioning model |
| `--select-model` | `gemma3:4b-it-qat` | selection model |
| `--force` | `false` | ignore cache |
| `--shader` | `{none}` | shader to put on background images (i.e. liquid_flow or wave_displace) |

### Debug helpers
```
clippers transcribe -a clip.mp3              # only whisper → stdout
clippers caption-images -i ./imgs            # only llava captions
clippers preview -a clip.mp3 -i ./imgs     # show planned timeline
clippers render -a clip.mp3 -i ./imgs     # re-render from cached timeline
```

---

## Workflow Example

```bash
# 1. check everything is reachable
clippers run -a demo.mp3 -i ./photos --aspects 1x1

# 2. not happy with image choices? tweak prompt or seed and re-run
clippers run -a demo.mp3 -i ./photos --force

# 3. want 16:9 only for YouTube?
clippers render -a demo.mp3 -i ./photos --aspects 16x9
```

---

## Prompt Customisation (Advanced)

The prompts are hard-coded for the MVP. Edit these files if you need tighter captions or different selection logic:

- **Caption prompt** – `internal/pipeline/caption.go` (`captionPrompt`)  
- **Selection prompt** – `internal/pipeline/plan.go` (`buildSelectionPrompt`)

---

## Cache Layout

```
.work/<hash>/
├── project.json        # inputs & settings hash
├── audio.wav           # 16 kHz mono
├── transcript.json     # whisper output
├── images/
│   ├── captions.json   # llava results
├── text/
│   └── windows.json    # 5s windows
├── timeline.json       # final shot list
├── subtitles.srt       # chunked subs
└── render/
    ├── concat_1x1.txt
    ├── concat_16x9.txt
    └── concat_9x16.txt
```

Delete `.work` or use `--force` to regenerate any stage.

---

## Troubleshooting

| symptom | fix |
|---------|-----|
| `ffmpeg` not found | add ffmpeg bin directory to PATH |
| whisper empty output | place `ggml-medium.en.bin` in `models/` or whisper search path |
| Ollama unreachable | `ollama serve` must stay running in its own terminal |
| CUDA OOM | use smaller whisper model (`base.en`) or reduce `--min-shot` to get fewer images |
| subtitles off-screen | adjust `--font-size` or `--blur` values per aspect |

---

## Road-map / Non-Goals

**MVP (done)**  
✔ local whisper, llava, gemma3 
✔ 1:1 / 16:9 / 9:16 outputs  
✔ hard cuts, blur background, centered image, bottom-center subs  

---

## Licence

MIT – see LICENSE file.