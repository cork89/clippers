// ./internal/pipeline/preflight.go
package pipeline

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/cork89/clippers/internal/config"
	"github.com/cork89/clippers/internal/llm"
	"github.com/cork89/clippers/internal/workdir"
)

// Preflight verifies all dependencies are available
func Preflight(cfg *config.Config, wd *workdir.WorkDir) error {
	fmt.Println("==> Preflight checks")

	// Check ffmpeg
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return fmt.Errorf("ffmpeg not found in PATH: %w", err)
	}
	fmt.Println("  ✓ ffmpeg found")

	// Check ffprobe
	if _, err := exec.LookPath("ffprobe"); err != nil {
		return fmt.Errorf("ffprobe not found in PATH: %w", err)
	}
	fmt.Println("  ✓ ffprobe found")

	// Check whisper-ctranslate2
	whisperBin := findWhisper()
	if whisperBin == "" {
		return fmt.Errorf("whisper-ctranslate2 not found in PATH (tried: whisper-ctranslate2)")
	}
	fmt.Printf("  ✓ whisper-ctranslate2 found: %s\n", whisperBin)

	// Check SubtitleEdit
	subtitleEditBin := findSubtitleEdit()
	if subtitleEditBin == "" {
		return fmt.Errorf("SubtitleEdit not found in PATH or common locations")
	}
	fmt.Printf("  ✓ SubtitleEdit found: %s\n", subtitleEditBin)

	// Check LLM provider
	if err := preflightLLM(cfg, wd); err != nil {
		return err
	}

	// Check audio file exists
	if _, err := os.Stat(cfg.AudioPath); err != nil {
		return fmt.Errorf("audio file not found: %s", cfg.AudioPath)
	}
	fmt.Printf("  ✓ audio file: %s\n", cfg.AudioPath)

	// Check images directory exists and has images
	images, err := listImages(cfg.ImagesDir)
	if err != nil {
		return fmt.Errorf("failed to read images directory: %w", err)
	}
	if len(images) == 0 {
		return fmt.Errorf("no images found in directory: %s", cfg.ImagesDir)
	}
	fmt.Printf("  ✓ images directory: %s (%d images)\n", cfg.ImagesDir, len(images))

	// Create output directory
	if err := os.MkdirAll(cfg.OutputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}
	fmt.Printf("  ✓ output directory: %s\n", cfg.OutputDir)

	return nil
}

func preflightLLM(cfg *config.Config, wd *workdir.WorkDir) error {
	if !needsLLM(cfg, wd) {
		return nil
	}

	switch cfg.LLMProvider {
	case config.LLMProviderOpenRouter:
		apiKey := os.Getenv("OPENROUTER_API_KEY")
		if apiKey == "" {
			return fmt.Errorf("OPENROUTER_API_KEY not set in .env file")
		}
		fmt.Println("  ✓ OpenRouter API key found")
		return nil
	default:
		client := llm.NewOllamaClient(cfg.OllamaHost)

		if err := client.Ping(); err != nil {
			return fmt.Errorf("ollama not reachable: %w\n  Make sure Ollama is running: ollama serve", err)
		}
		fmt.Printf("  ✓ ollama reachable at %s\n", cfg.OllamaHost)

		ollamaClient := client
		if ok, _ := ollamaClient.HasModel(cfg.VisionModel); !ok {
			return fmt.Errorf("vision model missing: %s", cfg.VisionModel)
		}

		if ok, _ := ollamaClient.HasModel(cfg.SelectModel); !ok {
			return fmt.Errorf("select model missing: %s", cfg.SelectModel)
		}

		return nil
	}
}

func findWhisper() string {
	names := []string{"whisper-ctranslate2"}
	for _, name := range names {
		if path, err := exec.LookPath(name); err == nil {
			return path
		}
	}
	return ""
}

func listImages(dir string) ([]string, error) {
	var images []string

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory: %w", err)
	}

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := filepath.Ext(e.Name())
		switch ext {
		case ".jpg", ".jpeg", ".png", ".webp", ".bmp", ".JPG", ".JPEG", ".PNG":
			images = append(images, filepath.Join(dir, e.Name()))
		}
	}

	return images, nil
}

func needsLLM(cfg *config.Config, wd *workdir.WorkDir) bool {
	if cfg.Force {
		return true
	}

	// Captioning cache
	if !wd.Exists("images/captions.json") {
		return true
	}

	// Timeline cache
	if !wd.Exists("timeline.json") {
		return true
	}

	return false
}
