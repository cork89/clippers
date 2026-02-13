package pipeline

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/cork89/clippers/internal/config"
	"github.com/cork89/clippers/internal/database"
	"github.com/cork89/clippers/internal/types"
	"github.com/cork89/clippers/internal/workdir"
)

func Transcribe(ctx context.Context, wd *workdir.WorkDir, cfg *config.Config, db *database.DB, force bool) (*types.Transcript, error) {
	if !force {
		exists, _ := db.Queries.TranscriptExists(ctx, wd.ProjectID())
		if exists == 1 {
			fmt.Println("==> Transcription (cached)")
			return db.GetFullTranscript(ctx, wd.ProjectID())
		}
	}

	fmt.Println("==> Transcribing audio")

	whisperBin := findWhisper()
	if whisperBin == "" {
		return nil, fmt.Errorf("whisper-ctranslate2 not found")
	}

	audioPath := wd.Path("audio.wav")
	outputDir := wd.Path(".")

	cmd := exec.Command(whisperBin,
		audioPath,
		"--model", cfg.WhisperModel,
		"--device", "cuda",
		"--compute_type", "float16",
		"--vad_filter", "True",
		"--batched", "True",
		"--batch_size", "16",
		"--output_format", "srt",
		"--output_dir", outputDir,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("whisper-ctranslate2 failed: %w\n%s", err, string(output))
	}

	srtPath := filepath.Join(outputDir, "audio.srt")
	transcript, err := parseSRT(srtPath)
	if err != nil {
		return nil, fmt.Errorf("failed to parse SRT: %w", err)
	}

	duration, err := getAudioDuration(audioPath)
	if err != nil {
		if len(transcript.Segments) > 0 {
			duration = transcript.Segments[len(transcript.Segments)-1].End
		}
	}
	transcript.DurationSec = duration
	transcript.Language = "en"

	if err := db.SaveFullTranscript(ctx, wd.ProjectID(), transcript); err != nil {
		return nil, fmt.Errorf("failed to save transcript: %w", err)
	}

	os.Remove(srtPath)

	fmt.Printf("  ✓ Transcribed %d segments (%.1fs)\n", len(transcript.Segments), duration)
	return transcript, nil
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
