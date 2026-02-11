// ./internal/pipeline/transcribe.go
package pipeline

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/cork89/clippers/internal/config"
	"github.com/cork89/clippers/internal/types"
	"github.com/cork89/clippers/internal/workdir"
)

// Transcribe runs whisper.cpp on the normalized audio
func Transcribe(wd *workdir.WorkDir, cfg *config.Config, force bool) (*types.Transcript, error) {
	if !force && wd.Exists("transcript.json") {
		fmt.Println("==> Transcription (cached)")
		var t types.Transcript
		if err := wd.ReadJSON("transcript.json", &t); err != nil {
			return nil, fmt.Errorf("failed to read transcript: %w", err)
		}
		return &t, nil
	}

	fmt.Println("==> Transcribing audio")

	audioPath := wd.Path("audio.wav")
	outputBase := wd.Path("transcript")

	// Find whisper binary
	whisperBin := findWhisper()
	if whisperBin == "" {
		return nil, fmt.Errorf("whisper.cpp not found")
	}

	// Run whisper.cpp with SRT output
	cmd := exec.Command(whisperBin,
		"-m", findWhisperModel(cfg.WhisperModel),
		"-f", audioPath,
		"-osrt",
		"-of", outputBase,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("whisper.cpp failed: %w\n%s", err, string(output))
	}

	// Parse the SRT file
	srtPath := outputBase + ".srt"
	transcript, err := parseSRT(srtPath)
	if err != nil {
		return nil, fmt.Errorf("failed to parse SRT: %w", err)
	}

	// Get audio duration
	duration, err := getAudioDuration(audioPath)
	if err != nil {
		if len(transcript.Segments) > 0 {
			duration = transcript.Segments[len(transcript.Segments)-1].End
		}
	}
	transcript.DurationSec = duration
	transcript.Language = "en"

	if err := wd.WriteJSON("transcript.json", transcript); err != nil {
		return nil, fmt.Errorf("failed to write transcript: %w", err)
	}

	fmt.Printf("  ✓ Transcribed %d segments (%.1fs)\n", len(transcript.Segments), duration)
	return transcript, nil
}

func findWhisperModel(model string) string {
	home := os.Getenv("HOME")
	if home == "" {
		home = os.Getenv("USERPROFILE")
	}

	locations := []string{
		fmt.Sprintf("models/ggml-%s.bin", model),
		fmt.Sprintf("ggml-%s.bin", model),
		filepath.Join(home, ".cache", "whisper", fmt.Sprintf("ggml-%s.bin", model)),
		filepath.Join(home, "whisper.cpp", "models", fmt.Sprintf("ggml-%s.bin", model)),
	}

	for _, loc := range locations {
		if _, err := os.Stat(loc); err == nil {
			return loc
		}
	}

	return fmt.Sprintf("ggml-%s.bin", model)
}

func parseSRT(path string) (*types.Transcript, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open srt file: %w", err)
	}
	defer f.Close()

	var segments []types.Segment
	scanner := bufio.NewScanner(f)

	timeRegex := regexp.MustCompile(`(\d{2}):(\d{2}):(\d{2}),(\d{3})\s*-->\s*(\d{2}):(\d{2}):(\d{2}),(\d{3})`)

	var currentSegment *types.Segment
	var textLines []string

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if line == "" {
			if currentSegment != nil && len(textLines) > 0 {
				currentSegment.Text = strings.Join(textLines, " ")
				segments = append(segments, *currentSegment)
			}
			currentSegment = nil
			textLines = nil
			continue
		}

		if matches := timeRegex.FindStringSubmatch(line); matches != nil {
			start := parseTimestamp(matches[1], matches[2], matches[3], matches[4])
			end := parseTimestamp(matches[5], matches[6], matches[7], matches[8])
			currentSegment = &types.Segment{Start: start, End: end}
			continue
		}

		if _, err := strconv.Atoi(line); err == nil && currentSegment == nil {
			continue
		}

		if currentSegment != nil {
			textLines = append(textLines, line)
		}
	}

	if currentSegment != nil && len(textLines) > 0 {
		currentSegment.Text = strings.Join(textLines, " ")
		segments = append(segments, *currentSegment)
	}

	return &types.Transcript{Segments: segments}, scanner.Err()
}

func parseTimestamp(h, m, s, ms string) float64 {
	hours, _ := strconv.Atoi(h)
	minutes, _ := strconv.Atoi(m)
	seconds, _ := strconv.Atoi(s)
	millis, _ := strconv.Atoi(ms)

	return float64(hours)*3600 + float64(minutes)*60 + float64(seconds) + float64(millis)/1000
}

func getAudioDuration(path string) (float64, error) {
	cmd := exec.Command("ffprobe",
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "json",
		path,
	)

	output, err := cmd.Output()
	if err != nil {
		return 0, err
	}

	var result struct {
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
	}

	if err := json.Unmarshal(output, &result); err != nil {
		return 0, err
	}

	return strconv.ParseFloat(result.Format.Duration, 64)
}
