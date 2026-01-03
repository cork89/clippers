package pipeline

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/cork89/clippers/internal/config"
	"github.com/cork89/clippers/internal/types"
	"github.com/cork89/clippers/internal/workdir"
)

// RenderAll renders all requested aspect ratios
func RenderAll(wd *workdir.WorkDir, cfg *config.Config, timeline *types.Timeline, srtPath string) ([]string, error) {
	fmt.Printf("==> Rendering %d aspect ratio(s)\n", len(cfg.Aspects))

	var outputs []string

	for i, aspect := range cfg.Aspects {
		fmt.Printf("\n--- [%d/%d] Rendering %s ---\n", i+1, len(cfg.Aspects), aspect)

		output, err := Render(wd, cfg, timeline, srtPath, aspect)
		if err != nil {
			return outputs, fmt.Errorf("render failed for %s: %w", aspect, err)
		}

		outputs = append(outputs, output)
	}

	return outputs, nil
}

// Render creates the final video output for a single aspect ratio
func Render(wd *workdir.WorkDir, cfg *config.Config, timeline *types.Timeline, srtPath string, aspect string) (string, error) {
	aspectCfg := config.GetAspectConfig(aspect, cfg.FontSize)

	fmt.Printf("  Resolution: %dx%d\n", aspectCfg.Width, aspectCfg.Height)
	fmt.Printf("  Font size: %d, Margin: %d\n", aspectCfg.FontSize, aspectCfg.MarginV)

	// Create concat file for ffmpeg
	concatPath := wd.Path(fmt.Sprintf("render/concat_%s.txt", aspect))
	if err := writeConcatFile(concatPath, timeline); err != nil {
		return "", fmt.Errorf("failed to write concat file: %w", err)
	}

	// Generate aspect-specific SRT with appropriate line lengths
	aspectSrtPath := wd.Path(fmt.Sprintf("render/subtitles_%s.srt", aspect))
	if err := copyFile(srtPath, aspectSrtPath); err != nil {
		return "", fmt.Errorf("failed to copy SRT: %w", err)
	}

	// Output path
	baseName := strings.TrimSuffix(filepath.Base(cfg.AudioPath), filepath.Ext(cfg.AudioPath))
	outputPath := filepath.Join(cfg.OutputDir, fmt.Sprintf("%s_%s.mp4", baseName, aspect))

	// Build filter complex for blur background + centered foreground
	filter := buildFilterComplex(aspectCfg, cfg.BlurStrength, aspectSrtPath, cfg.FPS)

	// Build ffmpeg command
	args := []string{
		"-y",
		"-threads", "0",
		"-f", "concat",
		"-safe", "0",
		"-i", concatPath,
		"-i", wd.Path("audio.wav"),
		"-filter_complex", filter,
		"-map", "[outv]",
		"-map", "1:a",
		"-c:v", "libx264",
		"-preset", "ultrafast",
		"-crf", "23",
		"-c:a", "aac",
		"-b:a", "192k",
		"-r", fmt.Sprintf("%d", cfg.FPS),
		"-pix_fmt", "yuv420p",
		"-shortest",
		outputPath,
	}

	fmt.Println("  Running ffmpeg...")

	cmd := exec.Command("ffmpeg", args...)

	// Capture stderr for error reporting but don't spam stdout
	// var stderr strings.Builder
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		// Print last part of stderr for debugging
		// errOutput := stderr.String()
		// lines := strings.Split(errOutput, "\n")
		// if len(lines) > 20 {
		// 	lines = lines[len(lines)-20:]
		// }
		// return "", fmt.Errorf("ffmpeg failed: %w\n%s", err, strings.Join(lines, "\n"))
		return "", fmt.Errorf("ffmpeg failed: %w", err)
	}

	// Get file size
	info, _ := os.Stat(outputPath)
	sizeMB := float64(info.Size()) / (1024 * 1024)

	fmt.Printf("  ✓ Rendered: %s (%.1f MB)\n", outputPath, sizeMB)
	return outputPath, nil
}

func writeConcatFile(path string, timeline *types.Timeline) error {
	var sb strings.Builder

	for _, entry := range timeline.Entries {
		duration := entry.End - entry.Start

		// Use absolute path and forward slashes for ffmpeg compatibility
		absPath, err := filepath.Abs(entry.Image)
		if err != nil {
			absPath = entry.Image
		}
		// Convert to forward slashes for ffmpeg
		absPath = filepath.ToSlash(absPath)

		// Escape single quotes in paths for ffmpeg concat
		escapedPath := strings.ReplaceAll(absPath, "'", "'\\''")
		sb.WriteString(fmt.Sprintf("file '%s'\n", escapedPath))
		sb.WriteString(fmt.Sprintf("duration %.3f\n", duration))
	}

	// FFmpeg concat demuxer requires last file repeated
	if len(timeline.Entries) > 0 {
		lastEntry := timeline.Entries[len(timeline.Entries)-1]
		absPath, _ := filepath.Abs(lastEntry.Image)
		absPath = filepath.ToSlash(absPath)
		escapedPath := strings.ReplaceAll(absPath, "'", "'\\''")
		sb.WriteString(fmt.Sprintf("file '%s'\n", escapedPath))
	}

	return os.WriteFile(path, []byte(sb.String()), 0644)
}

func buildFilterComplex(aspectCfg config.AspectConfig, blur int, srtPath string, fps int) string {
	width := aspectCfg.Width
	height := aspectCfg.Height

	// Escape the SRT path for ffmpeg filter (Windows-compatible)
	absPath, _ := filepath.Abs(srtPath)
	escapedSRT := filepath.ToSlash(absPath)
	escapedSRT = strings.ReplaceAll(escapedSRT, ":", "\\:")
	escapedSRT = strings.ReplaceAll(escapedSRT, "'", "\\'")

	subtitleStyle := fmt.Sprintf(
		"FontSize=%d,"+
			"FontName=Arial,"+
			"PrimaryColour=&HFFFFFF,"+
			"SecondaryColour=&H000000,"+
			"OutlineColour=&H000000,"+
			"BackColour=&H80000000,"+
			"Bold=1,"+
			"Outline=2,"+
			"Shadow=1,"+
			"MarginV=%d,"+
			"Alignment=2",
		aspectCfg.FontSize,
		aspectCfg.MarginV,
	)

	// Filter chain:
	// 1. Set framerate FIRST to normalize image timing
	// 2. Split input into two streams
	// 3. Background: scale to cover, crop to exact size, blur
	// 4. Foreground: scale to fit (contain), pad to exact size with transparency
	// 5. Overlay foreground centered on background
	// 6. Burn subtitles

	filter := fmt.Sprintf(
		// Set framerate first to fix image timing issues
		"[0:v]fps=%d,split=2[bg][fg];"+

			// Background: scale to cover, crop to exact size, blur
			"[bg]scale=%d:%d:force_original_aspect_ratio=increase,"+
			"crop=%d:%d:(iw-%d)/2:(ih-%d)/2,"+
			"boxblur=%d:%d[bgblur];"+

			// Foreground: scale to fit (decrease to contain)
			"[fg]scale=%d:%d:force_original_aspect_ratio=decrease[fgscaled];"+

			// Pad foreground to exact canvas size (centers automatically)
			"[fgscaled]pad=%d:%d:(ow-iw)/2:(oh-ih)/2:color=black@0[fgpad];"+

			// Overlay foreground on background
			"[bgblur][fgpad]overlay=0:0:format=auto[composed];"+

			// Burn subtitles
			"[composed]subtitles='%s':force_style='%s'[outv]",

		// FPS arg
		fps,

		// Background scale & crop args
		width, height,
		width, height, width, height,
		blur, blur,

		// Foreground scale args
		width, height,

		// Foreground pad args
		width, height,

		// Subtitle args
		escapedSRT, subtitleStyle,
	)

	return filter
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0644)
}
