// ./internal/pipeline/pipeline.go
package pipeline

import (
	"fmt"
	"strings"

	"github.com/cork89/clippers/internal/config"
	"github.com/cork89/clippers/internal/workdir"
)

// Run executes the full pipeline
func Run(cfg *config.Config) error {
	// Initialize work directory
	wd, err := workdir.New(cfg)
	if err != nil {
		return fmt.Errorf("failed to create work directory: %w", err)
	}
	fmt.Printf("==> Work directory: %s\n", wd.Root)

	// Step 0: Preflight
	if err := Preflight(cfg, wd); err != nil {
		return fmt.Errorf("preflight failed: %w", err)
	}

	// Step 1: Normalize audio
	_, err = NormalizeAudio(wd, cfg.AudioPath, cfg.Force)
	if err != nil {
		return fmt.Errorf("audio normalization failed: %w", err)
	}

	// Step 2: Transcribe
	transcript, err := Transcribe(wd, cfg, cfg.Force)
	if err != nil {
		return fmt.Errorf("transcription failed: %w", err)
	}

	// Step 3: Caption images
	catalog, err := CaptionImages(wd, cfg, cfg.Force)
	if err != nil {
		return fmt.Errorf("image captioning failed: %w", err)
	}

	// Step 4: Build text windows
	windows, err := BuildTextWindows(wd, cfg, transcript, cfg.Force)
	if err != nil {
		return fmt.Errorf("text window building failed: %w", err)
	}

	// Step 5: Plan timeline with LLM
	timeline, err := PlanTimeline(wd, cfg, windows, catalog, cfg.Force)
	if err != nil {
		return fmt.Errorf("timeline planning failed: %w", err)
	}

	// Step 6: Generate subtitles (SRT)
	srtPath, err := GenerateSubtitles(wd, cfg, transcript, cfg.Force)
	if err != nil {
		return fmt.Errorf("subtitle generation failed: %w", err)
	}

	// Step 7: Convert to ASS and add rounded backgrounds
	assPaths, err := ConvertAndProcessSubtitles(wd, cfg, srtPath, cfg.Force)
	if err != nil {
		return fmt.Errorf("ASS conversion failed: %w", err)
	}

	// Step 8: Render all aspects
	outputs, err := RenderAll(wd, cfg, timeline, assPaths)
	if err != nil {
		return err
	}

	// Summary
	fmt.Println("\n" + strings.Repeat("=", 50))
	fmt.Println("==> Done! Generated videos:")
	for _, output := range outputs {
		fmt.Printf("    • %s\n", output)
	}
	fmt.Println(strings.Repeat("=", 50))

	return nil
}
