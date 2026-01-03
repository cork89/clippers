package pipeline

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
	ChosenImageID  string  `json:"chosen_image_id"`
	BackupImageID  string  `json:"backup_image_id"`
	Confidence     float64 `json:"confidence"`
	Reason         string  `json:"reason"`
	MatchesTitle   bool    `json:"matches_title"`
	MatchesSegment bool    `json:"matches_segment"`
}

const defaultImageName = "default.png"

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

		var texts []string
		for _, seg := range transcript.Segments {
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

// DetectDefaultImage checks if default.png exists in the images directory
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

	// Check for default image
	defaultImagePath := DetectDefaultImage(cfg.ImagesDir)
	hasDefault := defaultImagePath != ""
	if hasDefault {
		fmt.Printf("  ✓ Found default image: %s\n", filepath.Base(defaultImagePath))
	}

	// Show title if provided
	if cfg.Title != "" {
		fmt.Printf("  ✓ Using title context: %q\n", cfg.Title)
	}

	client := ollama.NewClient(cfg.OllamaHost)

	// Build image catalog summary (excluding default from regular selection)
	catalogSummary := buildCatalogSummaryWithTitle(catalog, cfg.Title, hasDefault)

	var entries []types.TimelineEntry
	previousImageID := ""

	// Track usage stats
	defaultUsageCount := 0
	titleMatchCount := 0

	for i, window := range windows.Windows {
		fmt.Printf("  [%d/%d] %.1fs-%.1fs... ", i+1, len(windows.Windows), window.Start, window.End)

		selection, err := selectImageForWindow(client, cfg, window, catalogSummary, previousImageID, hasDefault)
		if err != nil {
			fmt.Printf("failed: %v\n", err)
			selection = &SelectionResult{
				ChosenImageID: catalog.Images[i%len(catalog.Images)].ID,
				Confidence:    0.3,
				Reason:        "fallback due to error",
			}
		}

		// Apply default image logic
		chosenID := selection.ChosenImageID
		imagePath := ""

		// Check if we should use default image
		useDefault := false
		if hasDefault {
			// Use default if:
			// 1. Confidence is below threshold
			// 2. LLM explicitly chose default
			// 3. Chosen image doesn't exist
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
			// Avoid immediate repeats if possible
			if chosenID == previousImageID && selection.BackupImageID != "" && selection.BackupImageID != previousImageID {
				chosenID = selection.BackupImageID
			}

			// Verify the chosen image exists
			imagePath = findImagePath(catalog, chosenID)
			if imagePath == "" {
				// Fallback to default if available, otherwise first image
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

	// Apply smoothing pass
	entries = smoothTimeline(entries, catalog, defaultImagePath)

	timeline := &types.Timeline{Entries: entries}

	if err := wd.WriteJSON("timeline.json", timeline); err != nil {
		return nil, err
	}

	// Print stats
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

func buildCatalogSummaryWithTitle(catalog *ImageCatalog, title string, hasDefault bool) string {
	var sb strings.Builder

	if title != "" {
		sb.WriteString(fmt.Sprintf("VIDEO TITLE: %q\n", title))
		sb.WriteString("Images that match the title's theme should be strongly preferred.\n\n")
	}

	sb.WriteString("Available images:\n")

	for _, img := range catalog.Images {
		// Skip default image in regular listing
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

func selectImageForWindow(client *ollama.Client, cfg *config.Config, window TextWindow, catalog, previousID string, hasDefault bool) (*SelectionResult, error) {
	prompt := buildSelectionPromptWithTitle(cfg, window, catalog, previousID, hasDefault)

	response, err := client.GenerateText(cfg.SelectModel, prompt, true)
	if err != nil {
		return nil, err
	}

	return parseSelectionResponse(response)
}

func buildSelectionPromptWithTitle(cfg *config.Config, window TextWindow, catalog, previousID string, hasDefault bool) string {
	var sb strings.Builder

	sb.WriteString("You are selecting images for a video. Choose the best image for this audio segment.\n\n")

	// Title context (weighted heavily)
	if cfg.Title != "" {
		sb.WriteString(fmt.Sprintf("=== VIDEO TITLE (IMPORTANT) ===\n%q\n", cfg.Title))
		sb.WriteString("Images matching the title's theme should be STRONGLY preferred.\n\n")
	}

	// Audio segment
	sb.WriteString(fmt.Sprintf("=== AUDIO SEGMENT (%.1fs to %.1fs) ===\n", window.Start, window.End))
	if window.Text != "" {
		sb.WriteString(fmt.Sprintf("%q\n\n", window.Text))
	} else {
		sb.WriteString("[No speech in this segment]\n\n")
	}

	// Image catalog
	sb.WriteString("=== AVAILABLE IMAGES ===\n")
	sb.WriteString(catalog)
	sb.WriteString("\n")

	// Previous image context
	if previousID != "" {
		sb.WriteString(fmt.Sprintf("Previous shot used: %s (avoid immediate repeat unless best match)\n\n", previousID))
	}

	// Selection criteria
	sb.WriteString("=== SELECTION CRITERIA ===\n")
	sb.WriteString("1. TITLE MATCH (highest priority): Does the image fit the video's overall title/theme?\n")
	sb.WriteString("2. SEGMENT MATCH: Does the image match what's being said in this segment?\n")
	sb.WriteString("3. VARIETY: Avoid repeating the previous image unless it's clearly the best choice.\n")

	if hasDefault {
		sb.WriteString("4. DEFAULT: Use 'default.png' ONLY if no image matches well. Set confidence to 0.3 or lower.\n")
	}

	sb.WriteString("\n")

	// Output format
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

	// Clamp confidence to valid range
	if result.Confidence < 0 {
		result.Confidence = 0
	}
	if result.Confidence > 1 {
		result.Confidence = 1
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
func smoothTimeline(entries []types.TimelineEntry, catalog *ImageCatalog, defaultImagePath string) []types.TimelineEntry {
	if len(entries) < 3 {
		return entries
	}

	// Detect A-B-A patterns and replace middle with A
	// But don't smooth away from default if confidence was very low
	for i := 1; i < len(entries)-1; i++ {
		prev := entries[i-1].ImageID
		curr := entries[i].ImageID
		next := entries[i+1].ImageID

		// Skip smoothing if current is default with very low confidence
		// (it was chosen for a reason)
		if defaultImagePath != "" && curr == filepath.Base(defaultImagePath) && entries[i].Confidence < 0.3 {
			continue
		}

		// If we have A-B-A, change to A-A-A
		if prev == next && curr != prev {
			entries[i].ImageID = prev
			entries[i].Image = entries[i-1].Image
		}
	}

	return entries
}
