// ./internal/pipeline/render.go
package pipeline

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/cork89/clippers/internal/config"
	"github.com/cork89/clippers/internal/types"
	"github.com/cork89/clippers/internal/workdir"
)

// RenderAll renders all requested aspect ratios
func RenderAll(wd *workdir.WorkDir, cfg *config.Config, timeline *types.Timeline, subtitlePaths []types.SubtitleAspect) ([]string, error) {
	fmt.Printf("==> Rendering %d aspect ratio(s)\n", len(cfg.Aspects))

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

	// Output path
	baseName := strings.TrimSuffix(filepath.Base(cfg.AudioPath), filepath.Ext(cfg.AudioPath))
	outputPath := filepath.Join(cfg.OutputDir, fmt.Sprintf("%s_%s.mp4", baseName, aspect))

	// Pass 1: Convert images to video with blur background + centered foreground
	fmt.Println("  Pass 1: Building video with blur backgrounds...")
	if err := renderPass1(concatPath, intermediatePath, timeline, aspectCfg, cfg.BlurStrength); err != nil {
		return "", fmt.Errorf("pass 1 failed: %w", err)
	}

	// Pass 2: Add subtitles and audio
	fmt.Println("  Pass 2: Adding subtitles and audio...")
	if err := renderPass2(wd, intermediatePath, subtitlePath, outputPath, aspectCfg); err != nil {
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

func renderPass1(concatPath, outputPath string, timeline *types.Timeline, aspectCfg config.AspectConfig, blur int) error {
	width := aspectCfg.Width
	height := aspectCfg.Height

	var totalDuration float64
	if len(timeline.Entries) > 0 {
		totalDuration = timeline.Entries[len(timeline.Entries)-1].End
	}

	effectiveBlur := blur
	if effectiveBlur > 15 {
		effectiveBlur = 15
	}

	filter := fmt.Sprintf(
		// 1. Split input
		"[0:v]split=2[bg][fg];"+

			// 2. Blurred background
			"[bg]scale=%d:%d:force_original_aspect_ratio=increase,"+
			"crop=%d:%d:(iw-%d)/2:(ih-%d)/2,"+
			"boxblur=%d:%d[bgblur];"+

			// 3. Foreground - scale and center
			"[fg]scale=%d:%d:force_original_aspect_ratio=decrease[fgscaled];"+
			"[fgscaled]pad=%d:%d:(ow-iw)/2:(oh-ih)/2:color=black@0[fgpad];"+

			// 4. Composite foreground onto blurred background
			"[bgblur][fgpad]overlay=0:0:format=auto[composite];"+

			// 5. Generate shader - sweeping wave pattern
			"color=c=0x6644AA:s=%dx%d:d=%.2f,format=rgba,"+
			"geq="+
			"r='100+50*sin(2*PI*(X/%d*2-T*30))':"+
			"g='80+40*sin(2*PI*(Y/%d*2-T*20))':"+
			"b='150+50*sin(2*PI*((X+Y)/%d-T*40))':"+
			"a='50'[shader];"+

			// 6. Overlay shader ON TOP of the composite
			"[composite][shader]overlay=0:0:shortest=1:format=auto,"+
			"fps=24,format=yuv420p",

		// Background params
		width, height, width, height, width, height, effectiveBlur, effectiveBlur,
		// Foreground params
		width, height, width, height,
		// Shader params
		width, height, totalDuration,
		width,  // for horizontal wave
		height, // for vertical wave
		width,  // for diagonal wave (X+Y)/width
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
		"-shortest",
		outputPath,
	}

	cmd := exec.Command("ffmpeg", args...)
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// renderPass1 converts the image sequence to video with blur background + centered foreground
func renderPass1bkp(concatPath, outputPath string, aspectCfg config.AspectConfig, blur int) error {
	width := aspectCfg.Width
	height := aspectCfg.Height

	effectiveBlur := blur
	if effectiveBlur > 15 {
		effectiveBlur = 15
	}

	// Inside renderPass1...

	// 1. We define our 'Shader' (The "Math" moving background)
	// This creates a pulsing purple/blue wave

	// Use a simpler 'color' filter first to verify the overlay works,
	// then we can swap in the complex 'geq' math.
	// Notice the 'f=lavfi' and the escaped colons.
	shaderSource := fmt.Sprintf("color=c=blue@0.5:s=%dx%d:d=1,f=lavfi", width, height)

	filter := fmt.Sprintf(
		"[0:v]split=2[bg][fg];"+
			"[bg]scale=%d:%d:force_original_aspect_ratio=increase,crop=%d:%d:(iw-%d)/2:(ih-%d)/2,boxblur=15[bgblur];"+
			// The key change: f=lavfi tells the movie filter to treat the string as a generator
			"movie='%s'[shader];"+
			"[bgblur][shader]overlay=format=auto[combined_bg];"+
			"[fg]scale=%d:%d:force_original_aspect_ratio=decrease[fgscaled];"+
			"[fgscaled]pad=%d:%d:(ow-iw)/2:(oh-ih)/2:color=black@0[fgpad];"+
			"[combined_bg][fgpad]overlay=0:0:format=auto,fps=24,format=yuv420p",
		width, height, width, height, width, height,
		shaderSource,
		width, height,
		width, height,
	)

	// Full filter: blur background + centered foreground (aspect ratio preserved)
	// filter := fmt.Sprintf(
	// 	"split=2[bg][fg];"+
	// 		// Background: scale to cover, crop to exact size, blur
	// 		"[bg]scale=%d:%d:force_original_aspect_ratio=increase,"+
	// 		"crop=%d:%d:(iw-%d)/2:(ih-%d)/2,"+
	// 		"boxblur=%d:%d[bgblur];"+
	// 		// Foreground: scale to fit (maintains aspect ratio)
	// 		"[fg]scale=%d:%d:force_original_aspect_ratio=decrease[fgscaled];"+
	// 		// Pad foreground to center it (transparent padding)
	// 		"[fgscaled]pad=%d:%d:(ow-iw)/2:(oh-ih)/2:color=black@0[fgpad];"+
	// 		// Overlay centered foreground on blurred background
	// 		"[bgblur][fgpad]overlay=0:0:format=auto,"+
	// 		// Convert to constant framerate
	// 		"fps=24,format=yuv420p",
	// 	width, height,
	// 	width, height, width, height,
	// 	effectiveBlur, effectiveBlur,
	// 	width, height,
	// 	width, height,
	// )

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
func renderPass2(wd *workdir.WorkDir, videoPath, subtitlePath, outputPath string, aspectCfg config.AspectConfig) error {
	absPath, _ := filepath.Abs(subtitlePath)
	escapedSubtitle := filepath.ToSlash(absPath)
	escapedSubtitle = strings.ReplaceAll(escapedSubtitle, ":", "\\:")
	escapedSubtitle = strings.ReplaceAll(escapedSubtitle, "'", "\\'")

	// Determine filter based on subtitle type
	var filter string
	if strings.HasSuffix(strings.ToLower(subtitlePath), ".ass") {
		// ASS files have their own styling, use ass filter
		filter = fmt.Sprintf("ass='%s'", escapedSubtitle)
	} else {
		// SRT files need style override
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

	absSub, _ := filepath.Abs(subtitlePath)
	fmt.Printf("  DEBUG: subtitle path = %s\n", absSub)
	if _, err := os.Stat(absSub); os.IsNotExist(err) {
		fmt.Printf("  DEBUG: subtitle file NOT FOUND\n")
	} else {
		fmt.Printf("  DEBUG: subtitle file exists, size %d bytes\n", mustGetSize(absSub))
	}
	// inside renderPass2(), just before building the FFmpeg command
	fmt.Printf("  DEBUG: video duration ~%.1f s\n", getVideoDuration(videoPath))
	fmt.Printf("  DEBUG: first+last subtitle line from ASS:\n")
	printFirstLastASS(subtitlePath)

	return cmd.Run()
}

func getVideoDuration(p string) float64 {
	cmd := exec.Command("ffprobe", "-v", "error",
		"-show_entries", "format=duration", "-of", "csv=p=0", p)
	out, _ := cmd.Output()
	f, _ := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	return f
}

func printFirstLastASS(ass string) {
	b, _ := os.ReadFile(ass)
	lines := strings.Split(string(b), "\n")
	inEvents := false
	var first, last string
	for _, l := range lines {
		txt := strings.TrimSpace(l)
		if txt == "[Events]" {
			inEvents = true
			continue
		}
		if !inEvents || !strings.HasPrefix(txt, "Dialogue:") {
			continue
		}
		if first == "" {
			first = txt
		}
		last = txt
	}
	fmt.Printf("    first: %s\n", first)
	fmt.Printf("    last:  %s\n", last)
}

func mustGetSize(p string) int64 { i, _ := os.Stat(p); return i.Size() }

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
