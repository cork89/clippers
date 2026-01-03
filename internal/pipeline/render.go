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

	// Generate aspect-specific SRT
	aspectSrtPath := wd.Path(fmt.Sprintf("render/subtitles_%s.srt", aspect))
	if err := copyFile(srtPath, aspectSrtPath); err != nil {
		return "", fmt.Errorf("failed to copy SRT: %w", err)
	}

	// Intermediate video path (images with blur background, no subtitles)
	intermediatePath := wd.Path(fmt.Sprintf("render/intermediate_%s.mp4", aspect))

	// Output path
	baseName := strings.TrimSuffix(filepath.Base(cfg.AudioPath), filepath.Ext(cfg.AudioPath))
	outputPath := filepath.Join(cfg.OutputDir, fmt.Sprintf("%s_%s.mp4", baseName, aspect))

	// Pass 1: Convert images to video with blur background + centered foreground
	fmt.Println("  Pass 1: Building video with blur backgrounds...")
	if err := renderPass1(concatPath, intermediatePath, aspectCfg, cfg.BlurStrength); err != nil {
		return "", fmt.Errorf("pass 1 failed: %w", err)
	}

	// Pass 2: Add subtitles and audio
	fmt.Println("  Pass 2: Adding subtitles and audio...")
	if err := renderPass2(wd, intermediatePath, aspectSrtPath, outputPath, aspectCfg); err != nil {
		return "", fmt.Errorf("pass 2 failed: %w", err)
	}

	// Clean up intermediate file
	os.Remove(intermediatePath)

	// Get file size
	info, _ := os.Stat(outputPath)
	sizeMB := float64(info.Size()) / (1024 * 1024)

	fmt.Printf("  ✓ Rendered: %s (%.1f MB)\n", outputPath, sizeMB)
	return outputPath, nil
}

// renderPass1 converts the image sequence to video with blur background + centered foreground
func renderPass1(concatPath, outputPath string, aspectCfg config.AspectConfig, blur int) error {
	width := aspectCfg.Width
	height := aspectCfg.Height

	effectiveBlur := blur
	if effectiveBlur > 15 {
		effectiveBlur = 15
	}

	// Full filter: blur background + centered foreground (aspect ratio preserved)
	filter := fmt.Sprintf(
		"split=2[bg][fg];"+
			// Background: scale to cover, crop to exact size, blur
			"[bg]scale=%d:%d:force_original_aspect_ratio=increase,"+
			"crop=%d:%d:(iw-%d)/2:(ih-%d)/2,"+
			"boxblur=%d:%d[bgblur];"+
			// Foreground: scale to fit (maintains aspect ratio)
			"[fg]scale=%d:%d:force_original_aspect_ratio=decrease[fgscaled];"+
			// Pad foreground to center it (transparent padding)
			"[fgscaled]pad=%d:%d:(ow-iw)/2:(oh-ih)/2:color=black@0[fgpad];"+
			// Overlay centered foreground on blurred background
			"[bgblur][fgpad]overlay=0:0:format=auto,"+
			// Convert to constant framerate
			"fps=24,format=yuv420p",
		width, height,
		width, height, width, height,
		effectiveBlur, effectiveBlur,
		width, height,
		width, height,
	)

	args := []string{
		"-y",
		"-f", "concat",
		"-safe", "0",
		"-i", concatPath,
		"-filter_complex", filter,
		"-c:v", "libx264",
		"-preset", "ultrafast",
		"-crf", "18",
		outputPath,
	}

	cmd := exec.Command("ffmpeg", args...)
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// renderPass2 adds subtitles and combines with audio
func renderPass2(wd *workdir.WorkDir, videoPath, srtPath, outputPath string, aspectCfg config.AspectConfig) error {
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

	// Just add subtitles - video already has blur background
	filter := fmt.Sprintf("subtitles='%s':force_style='%s'", escapedSRT, subtitleStyle)

	args := []string{
		"-y",
		"-i", videoPath,
		"-i", wd.Path("audio.wav"),
		"-vf", filter,
		"-map", "0:v",
		"-map", "1:a",
		"-c:v", "libx264",
		"-preset", "fast",
		"-crf", "23",
		"-threads", "0",
		"-c:a", "aac",
		"-b:a", "192k",
		"-shortest",
		"-stats",
		outputPath,
	}

	cmd := exec.Command("ffmpeg", args...)
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

func writeConcatFile(path string, timeline *types.Timeline) error {
	var sb strings.Builder

	for _, entry := range timeline.Entries {
		duration := entry.End - entry.Start

		absPath, err := filepath.Abs(entry.Image)
		if err != nil {
			absPath = entry.Image
		}
		absPath = filepath.ToSlash(absPath)

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

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0644)
}
