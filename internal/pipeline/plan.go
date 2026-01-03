package pipeline

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cork89/clippers/internal/config"
	"github.com/cork89/clippers/internal/ollama"
	"github.com/cork89/clippers/internal/types"
	"github.com/cork89/clippers/internal/workdir"
)

// TextWindow represents a time window with its text content
type TextWindow struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
	Text  string  `json:"text"`
}

// TextWindows holds all windows
type TextWindows struct {
	Windows []TextWindow `json:"windows"`
}

// SelectionResult is the LLM's response for image selection
type SelectionResult struct {
	ChosenImageID string  `json:"chosen_image_id"`
	BackupImageID string  `json:"backup_image_id"`
	Confidence    float64 `json:"confidence"`
	Reason        string  `json:"reason"`
}

// BuildTextWindows creates fixed-duration windows with overlapping text
func BuildTextWindows(wd *workdir.WorkDir, cfg *config.Config, transcript *types.Transcript, force bool) (*TextWindows, error) {
	if !force && wd.Exists("text/windows.json") {
		fmt.Println("==> Text windows (cached)")
		var windows TextWindows
		if err := wd.ReadJSON("text/windows.json", &windows); err != nil {
			return nil, err
		}
		return &windows, nil
	}

	fmt.Println("==> Building text windows")

	duration := transcript.DurationSec
	windowSize := cfg.MinShotSec

	var windows []TextWindow
	currentTime := 0.0

	for currentTime < duration {
		end := currentTime + windowSize
		if end > duration {
			end = duration
		}

		// Collect text from overlapping segments
		var texts []string
		for _, seg := range transcript.Segments {
			// Check if segment overlaps with window
			if seg.End > currentTime && seg.Start < end {
				texts = append(texts, strings.TrimSpace(seg.Text))
			}
		}

		windows = append(windows, TextWindow{
			Start: currentTime,
			End:   end,
			Text:  strings.Join(texts, " "),
		})

		currentTime = end
	}

	result := &TextWindows{Windows: windows}

	if err := wd.WriteJSON("text/windows.json", result); err != nil {
		return nil, err
	}

	fmt.Printf("  ✓ Created %d windows (%.1fs each)\n", len(windows), windowSize)
	return result, nil
}

// PlanTimeline uses LLM to select images for each window
func PlanTimeline(wd *workdir.WorkDir, cfg *config.Config, windows *TextWindows, catalog *ImageCatalog, force bool) (*types.Timeline, error) {
	if !force && wd.Exists("timeline.json") {
		fmt.Println("==> Timeline planning (cached)")
		var timeline types.Timeline
		if err := wd.ReadJSON("timeline.json", &timeline); err != nil {
			return nil, err
		}
		return &timeline, nil
	}

	fmt.Println("==> Planning timeline with", cfg.SelectModel)

	client := ollama.NewClient(cfg.OllamaHost)

	// Build image catalog summary for the prompt
	catalogSummary := buildCatalogSummary(catalog)

	var entries []types.TimelineEntry
	previousImageID := ""

	for i, window := range windows.Windows {
		fmt.Printf("  [%d/%d] %.1fs-%.1fs... ", i+1, len(windows.Windows), window.Start, window.End)

		selection, err := selectImageForWindow(client, cfg.SelectModel, window, catalogSummary, previousImageID)
		if err != nil {
			fmt.Printf("failed: %v\n", err)
			// Fallback: use first image or cycle
			selection = &SelectionResult{
				ChosenImageID: catalog.Images[i%len(catalog.Images)].ID,
				Confidence:    0.5,
				Reason:        "fallback selection",
			}
		} else {
			fmt.Printf("✓ %s\n", selection.ChosenImageID)
		}

		// Apply post-processing rules
		chosenID := selection.ChosenImageID

		// Avoid immediate repeats if possible
		if chosenID == previousImageID && selection.BackupImageID != "" && selection.BackupImageID != previousImageID {
			chosenID = selection.BackupImageID
		}

		// Verify the chosen image exists
		imagePath := findImagePath(catalog, chosenID)
		if imagePath == "" {
			// Fallback to first available
			chosenID = catalog.Images[0].ID
			imagePath = catalog.Images[0].Path
		}

		entries = append(entries, types.TimelineEntry{
			Start:   window.Start,
			End:     window.End,
			ImageID: chosenID,
			Image:   imagePath,
		})

		previousImageID = chosenID
	}

	// Apply smoothing pass to avoid rapid back-and-forth
	entries = smoothTimeline(entries, catalog)

	timeline := &types.Timeline{Entries: entries}

	if err := wd.WriteJSON("timeline.json", timeline); err != nil {
		return nil, err
	}

	fmt.Printf("  ✓ Planned %d shots\n", len(entries))
	return timeline, nil
}

func buildCatalogSummary(catalog *ImageCatalog) string {
	var sb strings.Builder
	sb.WriteString("Available images:\n")

	for _, img := range catalog.Images {
		tags := strings.Join(img.Tags, ", ")
		sb.WriteString(fmt.Sprintf("- ID: %s\n  Caption: %s\n  Tags: %s\n\n", img.ID, img.Caption, tags))
	}

	return sb.String()
}

func selectImageForWindow(client *ollama.Client, model string, window TextWindow, catalog, previousID string) (*SelectionResult, error) {
	prompt := buildSelectionPrompt(window, catalog, previousID)

	response, err := client.GenerateText(model, prompt, true)
	if err != nil {
		return nil, err
	}

	return parseSelectionResponse(response)
}

func buildSelectionPrompt(window TextWindow, catalog, previousID string) string {
	previousNote := ""
	if previousID != "" {
		previousNote = fmt.Sprintf("\nThe previous shot used image ID: %s. Try to avoid using the same image immediately unless it's clearly the best match.", previousID)
	}

	return fmt.Sprintf(`You are selecting images for a video. Choose the best image to display during this audio segment.

Audio text for this segment (%.1fs to %.1fs):
"%s"

%s%s

Respond with ONLY valid JSON in this exact format:
{
  "chosen_image_id": "filename.jpg",
  "backup_image_id": "other_filename.jpg",
  "confidence": 0.8,
  "reason": "Brief explanation of why this image matches the audio"
}

Choose the image whose caption and tags best match the themes, subjects, or mood of the audio text.
The chosen_image_id must exactly match one of the image IDs listed above.

ONLY JSON. Nothing else.`, window.Start, window.End, window.Text, catalog, previousNote)
}

func parseSelectionResponse(response string) (*SelectionResult, error) {
	// Clean up response
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

	return &result, nil
}

func findImagePath(catalog *ImageCatalog, imageID string) string {
	for _, img := range catalog.Images {
		if img.ID == imageID {
			return img.Path
		}
	}
	return ""
}

// smoothTimeline reduces rapid back-and-forth between images
func smoothTimeline(entries []types.TimelineEntry, catalog *ImageCatalog) []types.TimelineEntry {
	if len(entries) < 3 {
		return entries
	}

	// Detect A-B-A patterns and replace middle with A
	for i := 1; i < len(entries)-1; i++ {
		prev := entries[i-1].ImageID
		curr := entries[i].ImageID
		next := entries[i+1].ImageID

		// If we have A-B-A, change to A-A-A
		if prev == next && curr != prev {
			entries[i].ImageID = prev
			entries[i].Image = entries[i-1].Image
		}
	}

	return entries
}
