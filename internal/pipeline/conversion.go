// ./internal/pipeline/conversion.go
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

	// Step 1: Convert SRT to ASS using SubtitleEdit
	rawAssPath := wd.Path("subtitles_raw.ass")
	if err := convertSRTtoASS(srtPath, rawAssPath); err != nil {
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

	cmd := exec.Command(subtitleEditBin,
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

// findSubtitleEdit looks for SubtitleEdit in common locations
func findSubtitleEdit() string {
	names := []string{
		"SubtitleEdit",
		"subtitleedit",
		"SubtitleEdit.exe",
	}

	for _, name := range names {
		if path, err := exec.LookPath(name); err == nil {
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
