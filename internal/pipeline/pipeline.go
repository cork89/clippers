package pipeline

import (
	"context"
	"fmt"
	"strings"

	"github.com/cork89/clippers/internal/config"
	"github.com/cork89/clippers/internal/database"
	"github.com/cork89/clippers/internal/workdir"
)

func Run(ctx context.Context, cfg *config.Config, db *database.DB) error {
	wd, err := workdir.New(ctx, cfg, db)
	if err != nil {
		return fmt.Errorf("failed to create work directory: %w", err)
	}
	fmt.Printf("==> Work directory: %s\n", wd.Root)

	if err := Preflight(cfg, wd); err != nil {
		return fmt.Errorf("preflight failed: %w", err)
	}

	_, err = NormalizeAudio(wd, cfg.AudioPath, cfg.Force)
	if err != nil {
		return fmt.Errorf("audio normalization failed: %w", err)
	}

	transcript, err := Transcribe(ctx, wd, cfg, db, cfg.Force)
	if err != nil {
		return fmt.Errorf("transcription failed: %w", err)
	}

	catalog, err := CaptionImages(ctx, wd, cfg, db, cfg.Force)
	if err != nil {
		return fmt.Errorf("image captioning failed: %w", err)
	}

	windows, err := BuildTextWindows(ctx, wd, cfg, db, transcript, cfg.Force)
	if err != nil {
		return fmt.Errorf("text window building failed: %w", err)
	}

	timeline, err := PlanTimeline(ctx, wd, cfg, db, windows, catalog, cfg.Force)
	if err != nil {
		return fmt.Errorf("timeline planning failed: %w", err)
	}

	srtPath, err := GenerateSubtitles(wd, cfg, transcript, timeline, cfg.Force)
	if err != nil {
		return fmt.Errorf("subtitle generation failed: %w", err)
	}

	assPaths, err := ConvertAndProcessSubtitles(wd, cfg, srtPath, cfg.Force)
	if err != nil {
		return fmt.Errorf("ASS conversion failed: %w", err)
	}

	outputs, err := RenderAll(wd, cfg, timeline, assPaths)
	if err != nil {
		return err
	}

	fmt.Println("\n" + strings.Repeat("=", 50))
	fmt.Println("==> Done! Generated videos:")
	for _, output := range outputs {
		fmt.Printf("    • %s\n", output)
	}
	fmt.Println(strings.Repeat("=", 50))

	return nil
}
