package pipeline

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/cork89/clippers/internal/config"
	"github.com/cork89/clippers/internal/ollama"
	"github.com/cork89/clippers/internal/workdir"
)

// ImageCaption holds the caption data for a single image
type ImageCaption struct {
	ID      string   `json:"id"`
	Path    string   `json:"path"`
	Caption string   `json:"caption"`
	Tags    []string `json:"tags"`
	Style   int      `json:"style"`
	Notes   string   `json:"notes,omitempty"`
}

// ImageCatalog holds all image captions
type ImageCatalog struct {
	Images []ImageCaption `json:"images"`
}

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

// CaptionImages generates captions for all images using Ollama vision model
func CaptionImages(wd *workdir.WorkDir, cfg *config.Config, force bool) (*ImageCatalog, error) {
	if !force && wd.Exists("images/captions.json") {
		fmt.Println("==> Image captioning (cached)")
		var catalog ImageCatalog
		if err := wd.ReadJSON("images/captions.json", &catalog); err != nil {
			return nil, err
		}
		return &catalog, nil
	}

	fmt.Println("==> Captioning images with", cfg.VisionModel)

	client := ollama.NewClient(cfg.OllamaHost)

	// Get list of images
	images, err := listImages(cfg.ImagesDir)
	if err != nil {
		return nil, fmt.Errorf("failed to list images: %w", err)
	}

	var catalog ImageCatalog

	for i, imagePath := range images {
		imageID := filepath.Base(imagePath)
		fmt.Printf("  [%d/%d] %s... ", i+1, len(images), imageID)

		caption, err := captionImage(client, cfg.VisionModel, imagePath)
		if err != nil {
			fmt.Printf("failed: %v\n", err)
			// Use fallback caption
			caption = &ImageCaption{
				ID:      imageID,
				Path:    imagePath,
				Caption: "An image",
				Tags:    []string{"image", "visual"},
				Style:   2,
			}
		} else {
			caption.ID = imageID
			caption.Path = imagePath
			fmt.Printf("✓\n")
		}

		catalog.Images = append(catalog.Images, *caption)
	}

	if err := wd.WriteJSON("images/captions.json", catalog); err != nil {
		return nil, err
	}

	fmt.Printf("  ✓ Captioned %d images\n", len(catalog.Images))
	return &catalog, nil
}

func captionImage(client *ollama.Client, model, imagePath string) (*ImageCaption, error) {
	// First attempt with JSON mode
	response, err := client.GenerateWithImage(model, captionPrompt, imagePath, true)
	if err != nil {
		return nil, err
	}

	caption, err := parseCaptionResponse(response)
	if err != nil {
		// Retry with stricter prompt
		response, err = client.GenerateWithImage(model, strictCaptionPrompt, imagePath, true)
		if err != nil {
			return nil, err
		}

		caption, err = parseCaptionResponse(response)
		if err != nil {
			return nil, fmt.Errorf("failed to parse caption after retry: %w", err)
		}
	}

	return caption, nil
}

func parseCaptionResponse(response string) (*ImageCaption, error) {
	// Clean up response - remove markdown code blocks if present
	response = strings.TrimSpace(response)
	response = strings.TrimPrefix(response, "```json")
	response = strings.TrimPrefix(response, "```")
	response = strings.TrimSuffix(response, "```")
	response = strings.TrimSpace(response)

	var caption ImageCaption
	if err := json.Unmarshal([]byte(response), &caption); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w (response: %s)", err, truncate(response, 200))
	}

	// Validate
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
