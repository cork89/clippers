package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cork89/clippers/internal/config"
	"github.com/cork89/clippers/internal/database"
	"github.com/cork89/clippers/internal/ollama"
	"github.com/cork89/clippers/internal/types"
	"github.com/cork89/clippers/internal/workdir"
)

type SelectionResult struct {
	ChosenImageID  string  `json:"chosen_image_id"`
	BackupImageID  string  `json:"backup_image_id"`
	Confidence     float64 `json:"confidence"`
	Reason         string  `json:"reason"`
	MatchesTitle   bool    `json:"matches_title"`
	MatchesSegment bool    `json:"matches_segment"`
}

func BuildTextWindows(ctx context.Context, wd *workdir.WorkDir, cfg *config.Config, db *database.DB, transcript *types.Transcript, force bool) ([]database.TextWindowData, error) {
	if !force {
		exists, _ := db.Queries.WindowsExist(ctx, wd.ProjectID())
		if exists == 1 {
			fmt.Println("==> Text windows (cached)")
			return db.GetTextWindows(ctx, wd.ProjectID())
		}
	}

	fmt.Println("==> Building text windows")

	duration := transcript.DurationSec
	windowSize := cfg.MinShotSec

	var windows []database.TextWindowData
	currentTime := 0.0

	for currentTime < duration {
		end := currentTime + windowSize
		if end > duration {
			end = duration
		}

		var texts []string
		for _, seg := range transcript.Segments {
			if seg.End > currentTime && seg.Start < end {
				texts = append(texts, strings.TrimSpace(seg.Text))
			}
		}

		windows = append(windows, database.TextWindowData{
			Start: currentTime,
			End:   end,
			Text:  strings.Join(texts, " "),
		})

		currentTime = end
	}

	if err := db.SaveTextWindows(ctx, wd.ProjectID(), windows); err != nil {
		return nil, fmt.Errorf("failed to save windows: %w", err)
	}

	fmt.Printf("  ✓ Created %d windows (%.1fs each)\n", len(windows), windowSize)
	return windows, nil
}

func DetectDefaultImage(imagesDir string) string {
	candidates := []string{
		"default.png",
		"default.jpg",
		"default.jpeg",
		"Default.png",
		"Default.jpg",
	}

	for _, name := range candidates {
		path := filepath.Join(imagesDir, name)
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	return ""
}

func PlanTimeline(ctx context.Context, wd *workdir.WorkDir, cfg *config.Config, db *database.DB, windows []database.TextWindowData, catalog *database.ImageCatalog, force bool) (*types.Timeline, error) {
	if !force {
		exists, _ := db.Queries.TimelineExists(ctx, wd.ProjectID())
		if exists == 1 {
			fmt.Println("==> Timeline planning (cached)")
			return db.GetTimeline(ctx, wd.ProjectID())
		}
	}

	fmt.Println("==> Planning timeline with", cfg.SelectModel)

	defaultImagePath := DetectDefaultImage(cfg.ImagesDir)
	hasDefault := defaultImagePath != ""
	if hasDefault {
		fmt.Printf("  ✓ Found default image: %s\n", filepath.Base(defaultImagePath))
	}

	if cfg.Title != "" {
		fmt.Printf("  ✓ Using title context: %q\n", cfg.Title)
	}

	client := ollama.NewClient(cfg.OllamaHost)

	catalogSummary := buildCatalogSummaryWithTitle(catalog, cfg.Title, hasDefault)

	var entries []types.TimelineEntry
	previousImageID := ""

	defaultUsageCount := 0
	titleMatchCount := 0

	for i, window := range windows {
		fmt.Printf("  [%d/%d] %.1fs-%.1fs... ", i+1, len(windows), window.Start, window.End)

		selection, err := selectImageForWindow(client, cfg, window, catalogSummary, previousImageID, hasDefault)
		if err != nil {
			fmt.Printf("failed: %v\n", err)
			selection = &SelectionResult{
				ChosenImageID: catalog.Images[i%len(catalog.Images)].ID,
				Confidence:    0.3,
				Reason:        "fallback due to error",
			}
		}

		chosenID := selection.ChosenImageID
		imagePath := ""

		useDefault := false
		if hasDefault {
			if selection.Confidence < cfg.DefaultImageWeight {
				useDefault = true
			}
			if strings.ToLower(chosenID) == "default.png" || strings.ToLower(chosenID) == "default" {
				useDefault = true
			}
		}

		if useDefault {
			chosenID = filepath.Base(defaultImagePath)
			imagePath = defaultImagePath
			defaultUsageCount++
			fmt.Printf("→ default (conf: %.2f)\n", selection.Confidence)
		} else {
			if chosenID == previousImageID && selection.BackupImageID != "" && selection.BackupImageID != previousImageID {
				chosenID = selection.BackupImageID
			}

			imagePath = findImagePath(catalog, chosenID)
			if imagePath == "" {
				if hasDefault {
					chosenID = filepath.Base(defaultImagePath)
					imagePath = defaultImagePath
					defaultUsageCount++
				} else {
					chosenID = catalog.Images[0].ID
					imagePath = catalog.Images[0].Path
				}
			}

			if selection.MatchesTitle {
				titleMatchCount++
			}
			fmt.Printf("✓ %s (conf: %.2f)\n", chosenID, selection.Confidence)
		}

		entries = append(entries, types.TimelineEntry{
			Start:      window.Start,
			End:        window.End,
			ImageID:    chosenID,
			Image:      imagePath,
			Confidence: selection.Confidence,
			Reason:     selection.Reason,
		})

		previousImageID = chosenID
	}

	entries = smoothTimeline(entries, catalog, defaultImagePath)

	timeline := &types.Timeline{Entries: entries}

	if err := db.SaveTimeline(ctx, wd.ProjectID(), timeline); err != nil {
		return nil, fmt.Errorf("failed to write timeline: %w", err)
	}

	fmt.Printf("  ✓ Planned %d shots\n", len(entries))
	if hasDefault {
		fmt.Printf("    Default image used: %d times (%.0f%%)\n",
			defaultUsageCount, float64(defaultUsageCount)/float64(len(entries))*100)
	}
	if cfg.Title != "" {
		fmt.Printf("    Title-matched shots: %d times (%.0f%%)\n",
			titleMatchCount, float64(titleMatchCount)/float64(len(entries))*100)
	}

	return timeline, nil
}

func buildCatalogSummaryWithTitle(catalog *database.ImageCatalog, title string, hasDefault bool) string {
	var sb strings.Builder

	if title != "" {
		sb.WriteString(fmt.Sprintf("VIDEO TITLE: %q\n", title))
		sb.WriteString("Images that match the title's theme should be strongly preferred.\n\n")
	}

	sb.WriteString("Available images:\n")

	for _, img := range catalog.Images {
		if strings.HasPrefix(strings.ToLower(img.ID), "default.") {
			continue
		}

		tags := strings.Join(img.Tags, ", ")
		sb.WriteString(fmt.Sprintf("- ID: %s\n  Caption: %s\n  Tags: %s\n\n", img.ID, img.Caption, tags))
	}

	if hasDefault {
		sb.WriteString("\nSPECIAL: default.png\n")
		sb.WriteString("  Use this when NO other image matches the audio segment well.\n")
		sb.WriteString("  Set confidence LOW (0.2-0.4) when using default.\n")
	}

	return sb.String()
}

func selectImageForWindow(client *ollama.Client, cfg *config.Config, window database.TextWindowData, catalog, previousID string, hasDefault bool) (*SelectionResult, error) {
	prompt := buildSelectionPromptWithTitle(cfg, window, catalog, previousID, hasDefault)

	response, err := client.GenerateText(cfg.SelectModel, prompt, true)
	if err != nil {
		return nil, fmt.Errorf("failed to generate text: %w", err)
	}

	return parseSelectionResponse(response)
}

func buildSelectionPromptWithTitle(cfg *config.Config, window database.TextWindowData, catalog, previousID string, hasDefault bool) string {
	var sb strings.Builder

	sb.WriteString("You are selecting images for a video. Choose the best image for this audio segment.\n\n")

	if cfg.Title != "" {
		sb.WriteString(fmt.Sprintf("=== VIDEO TITLE (IMPORTANT) ===\n%q\n", cfg.Title))
		sb.WriteString("Images matching the title's theme should be STRONGLY preferred.\n\n")
	}

	sb.WriteString(fmt.Sprintf("=== AUDIO SEGMENT (%.1fs to %.1fs) ===\n", window.Start, window.End))
	if window.Text != "" {
		sb.WriteString(fmt.Sprintf("%q\n\n", window.Text))
	} else {
		sb.WriteString("[No speech in this segment]\n\n")
	}

	sb.WriteString("=== AVAILABLE IMAGES ===\n")
	sb.WriteString(catalog)
	sb.WriteString("\n")

	if previousID != "" {
		sb.WriteString(fmt.Sprintf("Previous shot used: %s (avoid immediate repeat unless best match)\n\n", previousID))
	}

	sb.WriteString("=== SELECTION CRITERIA ===\n")
	sb.WriteString("1. TITLE MATCH (highest priority): Does the image fit the video's overall title/theme?\n")
	sb.WriteString("2. SEGMENT MATCH: Does the image match what's being said in this segment?\n")
	sb.WriteString("3. VARIETY: Avoid repeating the previous image unless it's clearly the best choice.\n")

	if hasDefault {
		sb.WriteString("4. DEFAULT: Use 'default.png' ONLY if no image matches well. Set confidence to 0.3 or lower.\n")
	}

	sb.WriteString("\n")

	sb.WriteString(`Respond with ONLY valid JSON:
{
  "chosen_image_id": "filename.jpg",
  "backup_image_id": "other_filename.jpg",
  "confidence": 0.8,
  "reason": "Brief explanation",
  "matches_title": true,
  "matches_segment": true
}

Confidence guide:
- 0.9-1.0: Perfect match for title AND segment
- 0.7-0.8: Good match for title OR segment
- 0.5-0.6: Weak match, but acceptable
- 0.3-0.4: Poor match, consider default
- 0.1-0.2: No match, use default

ONLY JSON. Nothing else.`)

	return sb.String()
}

func parseSelectionResponse(response string) (*SelectionResult, error) {
	response = strings.TrimSpace(response)
	response = strings.TrimPrefix(response, "```json")
	response = strings.TrimPrefix(response, "```")
	response = strings.TrimSuffix(response, "```")
	response = strings.TrimSpace(response)

	var result SelectionResult
	if err := json.Unmarshal([]byte(response), &result); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w (response: %s)", err, truncate(response, 200))
	}

	if result.ChosenImageID == "" {
		return nil, fmt.Errorf("empty chosen_image_id")
	}

	if result.Confidence < 0 {
		result.Confidence = 0
	}
	if result.Confidence > 1 {
		result.Confidence = 1
	}

	return &result, nil
}

func findImagePath(catalog *database.ImageCatalog, imageID string) string {
	for _, img := range catalog.Images {
		if img.ID == imageID {
			return img.Path
		}
	}
	return ""
}

func smoothTimeline(entries []types.TimelineEntry, catalog *database.ImageCatalog, defaultImagePath string) []types.TimelineEntry {
	if len(entries) < 3 {
		return entries
	}

	for i := 1; i < len(entries)-1; i++ {
		prev := entries[i-1].ImageID
		curr := entries[i].ImageID
		next := entries[i+1].ImageID

		if defaultImagePath != "" && curr == filepath.Base(defaultImagePath) && entries[i].Confidence < 0.3 {
			continue
		}

		if prev == next && curr != prev {
			entries[i].ImageID = prev
			entries[i].Image = entries[i-1].Image
		}
	}

	return entries
}
