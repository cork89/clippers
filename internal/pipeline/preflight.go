package pipeline

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/cork89/clippers/internal/config"
	"github.com/cork89/clippers/internal/ollama"
)

// Preflight verifies all dependencies are available
func Preflight(cfg *config.Config) error {
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

	// Check whisper.cpp (try common names)
	whisperBin := findWhisper()
	if whisperBin == "" {
		return fmt.Errorf("whisper.cpp not found in PATH (tried: whisper, whisper-cpp, main)")
	}
	fmt.Printf("  ✓ whisper.cpp found: %s\n", whisperBin)

	// Check SubtitleEdit
	subtitleEditBin := findSubtitleEdit()
	if subtitleEditBin == "" {
		return fmt.Errorf("SubtitleEdit not found in PATH or common locations")
	}
	fmt.Printf("  ✓ SubtitleEdit found: %s\n", subtitleEditBin)

	// Check Ollama
	client := ollama.NewClient(cfg.OllamaHost)
	if err := client.Ping(); err != nil {
		return fmt.Errorf("ollama not reachable: %w\n  Make sure Ollama is running: ollama serve", err)
	}
	fmt.Printf("  ✓ ollama reachable at %s\n", cfg.OllamaHost)

	// Check required models
	hasVision, err := client.HasModel(cfg.VisionModel)
	if err != nil {
		return fmt.Errorf("failed to check for vision model: %w", err)
	}
	if !hasVision {
		return fmt.Errorf("vision model not found: %s\n  Run: ollama pull %s", cfg.VisionModel, cfg.VisionModel)
	}
	fmt.Printf("  ✓ vision model: %s\n", cfg.VisionModel)

	hasSelect, err := client.HasModel(cfg.SelectModel)
	if err != nil {
		return fmt.Errorf("failed to check for select model: %w", err)
	}
	if !hasSelect {
		return fmt.Errorf("select model not found: %s\n  Run: ollama pull %s", cfg.SelectModel, cfg.SelectModel)
	}
	fmt.Printf("  ✓ select model: %s\n", cfg.SelectModel)

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

func findWhisper() string {
	names := []string{"whisper", "whisper-cli", "whisper-cpp", "main", "whisper.exe", "main.exe"}
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
		return nil, err
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
