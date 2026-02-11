// ./internal/pipeline/render.go
package pipeline

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/cork89/clippers/internal/config"
	"github.com/cork89/clippers/internal/types"
	"github.com/cork89/clippers/internal/workdir"
)

// RenderAll renders all requested aspect ratios
func RenderAll(wd *workdir.WorkDir, cfg *config.Config, timeline *types.Timeline, subtitlePaths []types.SubtitleAspect) ([]string, error) {
	fmt.Printf("==> Rendering %d aspect ratio(s)\n", len(cfg.Aspects))
	if cfg.Shader != config.ShaderNone {
		fmt.Printf("    Shader: %s\n", cfg.Shader)
	}

	var outputs []string

	for i, subtitleAspect := range subtitlePaths {
		fmt.Printf("\n--- [%d/%d] Rendering %s ---\n", i+1, len(cfg.Aspects), subtitleAspect.Aspect)

		output, err := Render(wd, cfg, timeline, subtitleAspect.Path, subtitleAspect.Aspect)
		if err != nil {
			return outputs, fmt.Errorf("render failed for %s: %w", subtitleAspect.Aspect, err)
		}

		outputs = append(outputs, output)
	}

	return outputs, nil
}

// Render creates the final video output for a single aspect ratio
func Render(wd *workdir.WorkDir, cfg *config.Config, timeline *types.Timeline, subtitlePath string, aspect string) (string, error) {
	aspectCfg := config.GetAspectConfig(aspect, cfg.FontSize)

	fmt.Printf("  Resolution: %dx%d\n", aspectCfg.Width, aspectCfg.Height)
	fmt.Printf("  Font size: %d, Margin: %d\n", aspectCfg.FontSize, aspectCfg.MarginV)

	// Create concat file for ffmpeg
	concatPath := wd.Path(fmt.Sprintf("render/concat_%s.txt", aspect))
	if err := writeConcatFile(concatPath, timeline); err != nil {
		return "", fmt.Errorf("failed to write concat file: %w", err)
	}

	// Intermediate video path (images with blur background, no subtitles)
	intermediatePath := wd.Path(fmt.Sprintf("render/intermediate_%s.mp4", aspect))

	// Shader-processed video path (if shader is applied)
	shaderPath := wd.Path(fmt.Sprintf("render/shader_%s.mp4", aspect))

	// Output path
	baseName := strings.TrimSuffix(filepath.Base(cfg.AudioPath), filepath.Ext(cfg.AudioPath))
	outputPath := filepath.Join(cfg.OutputDir, fmt.Sprintf("%s_%s.mp4", baseName, aspect))

	// Pass 1: Convert images to video with blur background + centered foreground
	fmt.Println("  Pass 1: Building video with blur backgrounds...")
	if err := renderPass1WithTransitions(timeline, intermediatePath, aspectCfg, cfg.BlurStrength); err != nil {
		return "", fmt.Errorf("pass 1 failed: %w", err)
	}

	// Pass 2 (optional): Apply shader effect
	videoForSubtitles := intermediatePath
	if cfg.Shader != config.ShaderNone {
		fmt.Printf("  Pass 2: Applying %s shader...\n", cfg.Shader)
		if err := renderShaderPass(cfg, intermediatePath, shaderPath, aspectCfg); err != nil {
			fmt.Printf("  Warning: shader pass failed (%v), continuing without shader\n", err)
		} else {
			videoForSubtitles = shaderPath
		}
	}

	// Final pass: Add subtitles and audio
	fmt.Println("  Final pass: Adding subtitles and audio...")
	if err := renderFinalPass(wd, videoForSubtitles, subtitlePath, outputPath, aspectCfg); err != nil {
		return "", fmt.Errorf("final pass failed: %w", err)
	}

	// Clean up intermediate files
	os.Remove(intermediatePath)
	if shaderPath != "" {
		os.Remove(shaderPath)
	}

	// Get file size
	info, err := os.Stat(outputPath)
	if err != nil {
		return outputPath, fmt.Errorf("failed to get output file size, %w", err)
	}

	sizeMB := float64(info.Size()) / (1024 * 1024)

	fmt.Printf("  ✓ Rendered: %s (%.1f MB)\n", outputPath, sizeMB)
	return outputPath, nil
}

func renderPass1WithTransitions(timeline *types.Timeline, outputPath string, aspectCfg config.AspectConfig, blur int) error {
	width := aspectCfg.Width
	height := aspectCfg.Height
	transitionDuration := 0.5 // duration of crossfade in seconds

	var args []string
	args = append(args, "-y")

	// 1. Add every image as an input
	for _, entry := range timeline.Entries {
		args = append(args, "-loop", "1", "-t", fmt.Sprintf("%.3f", entry.End-entry.Start+transitionDuration), "-i", entry.Image)
	}

	// 2. Build the complex filter
	var filter strings.Builder

	// Pre-process each input (Scale/Blur)
	for i := 0; i < len(timeline.Entries); i++ {
		fmt.Fprintf(&filter, "[%d:v]scale=%d:%d:force_original_aspect_ratio=increase,crop=%d:%d,boxblur=%d[v%d];",
			i, width, height, width, height, blur, i)
	}

	// Chain the xfades
	// Formula: offset = (end_of_prev_clip) - transition_duration
	lastStream := "v0"
	currentOffset := 0.0

	for i := 1; i < len(timeline.Entries); i++ {
		prevEntry := timeline.Entries[i-1]
		duration := prevEntry.End - prevEntry.Start
		currentOffset += duration

		nextStream := fmt.Sprintf("xf%d", i)
		fmt.Fprintf(&filter, "[%s][v%d]xfade=transition=fade:duration=%.3f:offset=%.3f[%s];",
			lastStream, i, transitionDuration, currentOffset, nextStream)
		lastStream = nextStream
	}

	filter.WriteString(fmt.Sprintf("[%s]format=yuv420p[outv]", lastStream))

	args = append(args, "-filter_complex", filter.String())
	args = append(args, "-map", "[outv]", "-c:v", "libx264", "-pix_fmt", "yuv420p", outputPath)

	cmd := exec.Command("ffmpeg", args...)
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// renderShaderPass applies a GLSL shader using libplacebo
func renderShaderPass(cfg *config.Config, inputPath, outputPath string, aspectCfg config.AspectConfig) error {
	shaderFile := filepath.Join(cfg.ShadersDir, string(cfg.Shader)+".glsl")

	// Check if shader file exists
	if _, err := os.Stat(shaderFile); os.IsNotExist(err) {
		return fmt.Errorf("shader file not found: %s", shaderFile)
	}

	absShaderPath, err := filepath.Abs(shaderFile)
	if err != nil {
		return err
	}

	// FFmpeg's filter parser needs colons and backslashes escaped if not quoted carefully.
	// Using ToSlash and then escaping colons for the filter string.
	cleanShaderPath := filepath.ToSlash(absShaderPath)
	escapedShaderPath := strings.ReplaceAll(cleanShaderPath, ":", "\\:")

	// Wrap the path in single quotes inside the filter description
	filter := fmt.Sprintf(
		"libplacebo=custom_shader_path='%s':w=%d:h=%d,format=yuv420p",
		escapedShaderPath, aspectCfg.Width, aspectCfg.Height,
	)

	var args []string

	// Use hardware acceleration on Windows
	if runtime.GOOS == "windows" {
		args = []string{
			"-y",
			"-init_hw_device", "d3d11va=d3d:0",
			"-filter_hw_device", "d3d",
			"-i", inputPath,
			"-vf", filter,
			"-c:v", "libx264",
			"-preset", "fast",
			"-crf", "20",
			"-pix_fmt", "yuv420p",
			outputPath,
		}
	} else {
		// Fallback for non-Windows (may need Vulkan or other setup)
		args = []string{
			"-y",
			"-i", inputPath,
			"-vf", filter,
			"-c:v", "libx264",
			"-preset", "fast",
			"-crf", "20",
			"-pix_fmt", "yuv420p",
			outputPath,
		}
	}

	cmd := exec.Command("ffmpeg", args...)
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// renderFinalPass adds subtitles and combines with audio
func renderFinalPass(wd *workdir.WorkDir, videoPath, subtitlePath, outputPath string, aspectCfg config.AspectConfig) error {
	absPath, _ := filepath.Abs(subtitlePath)
	escapedSubtitle := filepath.ToSlash(absPath)
	escapedSubtitle = strings.ReplaceAll(escapedSubtitle, ":", "\\:")
	escapedSubtitle = strings.ReplaceAll(escapedSubtitle, "'", "\\'")

	// Determine filter based on subtitle type
	var filter string
	if strings.HasSuffix(strings.ToLower(subtitlePath), ".ass") {
		filter = fmt.Sprintf("ass='%s'", escapedSubtitle)
	} else {
		subtitleStyle := fmt.Sprintf(
			"FontSize=%d,"+
				"FontName=AsapCondensed-Medium,"+
				"PrimaryColour=&HFFFFFF,"+
				"BackColour=&H80000000,"+
				"BorderStyle=4,"+
				"Outline=0,"+
				"Shadow=0,"+
				"Bold=1,"+
				"MarginV=%d,"+
				"Alignment=2",
			aspectCfg.FontSize,
			aspectCfg.MarginV,
		)
		filter = fmt.Sprintf("subtitles='%s':force_style='%s'", escapedSubtitle, subtitleStyle)
	}

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

	// Last file repeated (ffmpeg requirement)
	if len(timeline.Entries) > 0 {
		lastEntry := timeline.Entries[len(timeline.Entries)-1]
		absPath, _ := filepath.Abs(lastEntry.Image)
		absPath = filepath.ToSlash(absPath)
		escapedPath := strings.ReplaceAll(absPath, "'", "'\\''")
		sb.WriteString(fmt.Sprintf("file '%s'\n", escapedPath))
	}

	return os.WriteFile(path, []byte(sb.String()), 0644)
}
