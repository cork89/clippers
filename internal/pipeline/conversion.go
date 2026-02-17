// ./internal/pipeline/conversion.go
package pipeline

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/cork89/clippers/internal/config"
	"github.com/cork89/clippers/internal/types"
	"github.com/cork89/clippers/internal/workdir"
)

// ConvertAndProcessSubtitles converts SRT to ASS and applies rounded backgrounds
func ConvertAndProcessSubtitles(wd *workdir.WorkDir, cfg *config.Config, srtPath string, force bool) ([]types.SubtitleAspect, error) {
	subtitleAspects := make([]types.SubtitleAspect, 0)
	for _, aspect := range cfg.Aspects {
		processedPath := wd.Path(fmt.Sprintf("subtitles_%s.ass", aspect))
		subtitleAspects = append(subtitleAspects, types.SubtitleAspect{Aspect: aspect, Path: processedPath})
	}

	// Check if all aspect-specific ASS files exist
	allExist := true
	for _, sa := range subtitleAspects {
		if !wd.Exists(filepath.Base(sa.Path)) {
			allExist = false
			break
		}
	}

	if !force && allExist {
		fmt.Println("==> ASS conversion (cached)")
		return subtitleAspects, nil
	}

	fmt.Println("==> Converting subtitles to ASS")

	// Step 1: Convert SRT to ASS (SubtitleEdit by default, native Go behind flag)
	rawAssPath := wd.Path("subtitles_raw.ass")
	var err error
	if cfg.UseGoASSConversion {
		err = convertSRTtoASSGo(srtPath, rawAssPath)
	} else {
		err = convertSRTtoASS(srtPath, rawAssPath)
	}
	if err != nil {
		return subtitleAspects, fmt.Errorf("SRT to ASS conversion failed: %w", err)
	}
	fmt.Println("  ✓ Converted to ASS format")

	// Debug: print raw ASS content
	if rawContent, err := os.ReadFile(rawAssPath); err == nil {
		fmt.Printf("  DEBUG: Raw ASS file (%d bytes):\n", len(rawContent))
		lines := strings.Split(string(rawContent), "\n")
		for i, line := range lines {
			if i < 20 { // Print first 20 lines
				fmt.Printf("    %s\n", line)
			}
		}
	}

	for i, subtitleAspect := range subtitleAspects {
		fmt.Printf("\n--- [%d/%d] Processing %s ---\n", i+1, len(cfg.Aspects), subtitleAspect.Aspect)
		aspectCfg := config.GetAspectConfig(subtitleAspect.Aspect, cfg.FontSize)

		if err := addRoundedBackground(rawAssPath, subtitleAspect, aspectCfg); err != nil {
			return subtitleAspects, fmt.Errorf("ASS processing failed: %w", err)
		}
		fmt.Printf("  ✓ Processed %s\n", subtitleAspect.Aspect)
	}

	fmt.Println("  ✓ Added rounded backgrounds")

	return subtitleAspects, nil
}

// convertSRTtoASS uses SubtitleEdit CLI to convert SRT to ASS
func convertSRTtoASS(srtPath, assPath string) error {
	subtitleEditBin := findSubtitleEdit()
	if subtitleEditBin == "" {
		return fmt.Errorf("SubtitleEdit not found in PATH")
	}

	absSrt, err := filepath.Abs(srtPath)
	if err != nil {
		return err
	}

	outputDir := filepath.Dir(assPath)

	cmd := execCommand(subtitleEditBin,
		"/convert",
		absSrt,
		"AdvancedSubStationAlpha",
		fmt.Sprintf("/outputfolder:%s", outputDir),
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("SubtitleEdit failed: %w\n%s", err, string(output))
	}

	expectedOutput := filepath.Join(outputDir, replaceExt(filepath.Base(srtPath), ".ass"))

	if expectedOutput != assPath {
		if err := os.Rename(expectedOutput, assPath); err != nil {
			data, readErr := os.ReadFile(expectedOutput)
			if readErr != nil {
				return fmt.Errorf("failed to read converted file: %w", readErr)
			}
			if writeErr := os.WriteFile(assPath, data, 0644); writeErr != nil {
				return fmt.Errorf("failed to write ASS file: %w", writeErr)
			}
			os.Remove(expectedOutput)
		}
	}

	return nil
}

// convertSRTtoASSGo converts SRT to ASS without external tooling.
func convertSRTtoASSGo(srtPath, assPath string) error {
	transcript, err := parseSRT(srtPath)
	if err != nil {
		return fmt.Errorf("failed to parse srt: %w", err)
	}

	var b strings.Builder
	b.WriteString("[Script Info]\n")
	b.WriteString("ScriptType: v4.00+\n")
	b.WriteString("WrapStyle: 0\n")
	b.WriteString("ScaledBorderAndShadow: yes\n\n")

	b.WriteString("[V4+ Styles]\n")
	b.WriteString("Format: Name,Fontname,Fontsize,PrimaryColour,SecondaryColour,OutlineColour,BackColour,Bold,Italic,Underline,StrikeOut,ScaleX,ScaleY,Spacing,Angle,BorderStyle,Outline,Shadow,Alignment,MarginL,MarginR,MarginV,Encoding\n")
	b.WriteString("Style: Default,Arial,48,&H00FFFFFF,&H000000FF,&H00000000,&H80000000,0,0,0,0,100,100,0,0,1,2,0,2,20,20,20,1\n\n")

	b.WriteString("[Events]\n")
	b.WriteString("Format: Layer,Start,End,Style,Name,MarginL,MarginR,MarginV,Effect,Text\n")

	for _, seg := range transcript.Segments {
		text := strings.TrimSpace(seg.Text)
		if text == "" {
			continue
		}
		fmt.Fprintf(
			&b,
			"Dialogue: 0,%s,%s,Default,,0,0,0,,%s\n",
			formatASSTime(seg.Start),
			formatASSTime(seg.End),
			escapeASSText(text),
		)
	}

	if err := os.WriteFile(assPath, []byte(b.String()), 0644); err != nil {
		return fmt.Errorf("failed to write ass file: %w", err)
	}

	return nil
}

// findSubtitleEdit looks for SubtitleEdit in common locations
func findSubtitleEdit() string {
	names := []string{
		"SubtitleEdit",
		"subtitleedit",
		"SubtitleEdit.exe",
	}

	for _, name := range names {
		if path, err := execLookPath(name); err == nil {
			return path
		}
	}

	commonPaths := []string{
		`C:\Program Files\Subtitle Edit\SubtitleEdit.exe`,
		`C:\Program Files (x86)\Subtitle Edit\SubtitleEdit.exe`,
	}

	for _, p := range commonPaths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}

	return ""
}

func replaceExt(filename, newExt string) string {
	ext := filepath.Ext(filename)
	return filename[:len(filename)-len(ext)] + newExt
}

func formatASSTime(seconds float64) string {
	if seconds < 0 {
		seconds = 0
	}
	centis := int(math.Round(seconds * 100))
	h := centis / 360000
	m := (centis % 360000) / 6000
	s := (centis % 6000) / 100
	cs := centis % 100
	return fmt.Sprintf("%d:%02d:%02d.%02d", h, m, s, cs)
}

func escapeASSText(s string) string {
	escaped := strings.ReplaceAll(s, "\\", "\\\\")
	escaped = strings.ReplaceAll(escaped, "{", "\\{")
	escaped = strings.ReplaceAll(escaped, "}", "\\}")
	return escaped
}
