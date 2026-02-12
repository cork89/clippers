package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/cork89/clippers/internal/config"
	"github.com/cork89/clippers/internal/database"
	"github.com/cork89/clippers/internal/ollama"
	"github.com/cork89/clippers/internal/workdir"
)

const captionPrompt = `Analyze this image and respond with ONLY valid JSON in this exact format:
{
  "caption": "A single sentence describing the main subject and action in the image",
  "tags": ["tag1", "tag2", "tag3", "tag4", "tag5", "tag6", "tag7", "tag8"],
  "style": 3,
  "notes": "Optional notes about mood, colors, or composition"
}

Requirements:
- caption: One clear, descriptive sentence (10-20 words)
- tags: 8-15 relevant keywords (nouns, adjectives, themes, emotions, settings)
- style: Rate visual intensity from 0 (calm/minimal) to 5 (dramatic/complex)
- notes: Brief optional notes

Respond with ONLY the JSON object, no other text.`

const strictCaptionPrompt = `You must respond with ONLY a valid JSON object. No explanations, no markdown, no code blocks.

Analyze the image and output this exact JSON structure:
{"caption":"one sentence description","tags":["tag1","tag2","tag3","tag4","tag5","tag6","tag7","tag8"],"style":3,"notes":"optional"}

ONLY JSON. Nothing else.`

func CaptionImages(ctx context.Context, wd *workdir.WorkDir, cfg *config.Config, db *database.DB, force bool) (*database.ImageCatalog, error) {
	if !force {
		exists, _ := db.Queries.ImageCatalogExists(ctx, wd.ProjectID())
		if exists == 1 {
			fmt.Println("==> Image captioning (cached)")
			return db.GetImageCatalog(ctx, wd.ProjectID())
		}
	}

	fmt.Println("==> Captioning images with", cfg.VisionModel)

	client := ollama.NewClient(cfg.OllamaHost)

	images, err := listImages(cfg.ImagesDir)
	if err != nil {
		return nil, fmt.Errorf("failed to list images: %w", err)
	}

	defaultImagePath := DetectDefaultImage(cfg.ImagesDir)

	var catalog database.ImageCatalog
	catalog.DefaultImage = defaultImagePath

	for i, imagePath := range images {
		imageID := filepath.Base(imagePath)
		isDefault := strings.HasPrefix(strings.ToLower(imageID), "default.")

		fmt.Printf("  [%d/%d] %s", i+1, len(images), imageID)
		if isDefault {
			fmt.Print(" (default)")
		}
		fmt.Print("... ")

		caption, err := captionImage(client, cfg.VisionModel, imagePath)
		if err != nil {
			return nil, fmt.Errorf("Failed to caption image, %v", err)
		} else {
			caption.ID = imageID
			caption.Path = imagePath
			fmt.Printf("✓\n")
		}

		caption.IsDefault = isDefault

		if isDefault {
			caption.Tags = append(caption.Tags, "default", "fallback", "generic")
		}

		catalog.Images = append(catalog.Images, *caption)
	}

	if err := db.SaveImageCatalog(ctx, wd.ProjectID(), &catalog); err != nil {
		return nil, fmt.Errorf("failed to save captions: %w", err)
	}

	fmt.Printf("  ✓ Captioned %d images", len(catalog.Images))
	if defaultImagePath != "" {
		fmt.Printf(" (including default)")
	}
	fmt.Println()

	return &catalog, nil
}

func captionImage(client *ollama.Client, model, imagePath string) (*database.ImageCaption, error) {
	response, err := client.GenerateWithImage(model, captionPrompt, imagePath, true)
	if err != nil {
		return nil, fmt.Errorf("failed to generate captions: %w", err)
	}

	caption, err := parseCaptionResponse(response)
	if err != nil {
		response, err = client.GenerateWithImage(model, strictCaptionPrompt, imagePath, true)
		if err != nil {
			return nil, fmt.Errorf("failed to generate strict captions: %w", err)
		}

		caption, err = parseCaptionResponse(response)
		if err != nil {
			return nil, fmt.Errorf("failed to parse caption after retry: %w", err)
		}
	}

	return caption, nil
}

func parseCaptionResponse(response string) (*database.ImageCaption, error) {
	response = strings.TrimSpace(response)
	response = strings.TrimPrefix(response, "```json")
	response = strings.TrimPrefix(response, "```")
	response = strings.TrimSuffix(response, "```")
	response = strings.TrimSpace(response)

	var caption database.ImageCaption
	if err := json.Unmarshal([]byte(response), &caption); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w (response: %s)", err, truncate(response, 200))
	}

	if caption.Caption == "" {
		return nil, fmt.Errorf("empty caption")
	}
	if len(caption.Tags) == 0 {
		caption.Tags = []string{"image"}
	}

	return &caption, nil
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
