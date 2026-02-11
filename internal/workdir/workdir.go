package workdir

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/cork89/clippers/internal/config"
	"github.com/cork89/clippers/internal/types"
)

// WorkDir manages the working directory for a project
type WorkDir struct {
	Root string
	Hash string
	cfg  *config.Config
}

// New creates or opens a work directory based on input hashes
func New(cfg *config.Config) (*WorkDir, error) {
	audioHash, err := hashFile(cfg.AudioPath)
	if err != nil {
		return nil, fmt.Errorf("failed to hash audio: %w", err)
	}

	imagesHash, err := hashDirectory(cfg.ImagesDir)
	if err != nil {
		return nil, fmt.Errorf("failed to hash images: %w", err)
	}

	// Combine hashes for unique work directory
	combined := fmt.Sprintf("%s:%s:%.1f:%d", audioHash[:16], imagesHash[:16], cfg.MinShotSec, cfg.BlurStrength)
	h := sha256.Sum256([]byte(combined))
	hash := hex.EncodeToString(h[:])[:16]

	root := filepath.Join(cfg.WorkDir, hash)
	if err := os.MkdirAll(root, 0755); err != nil {
		return nil, fmt.Errorf("failed to create work directory: %w", err)
	}

	// Create subdirectories
	for _, sub := range []string{"images", "text", "render"} {
		if err := os.MkdirAll(filepath.Join(root, sub), 0755); err != nil {
			return nil, fmt.Errorf("failed to make work directory: %w", err)
		}
	}

	wd := &WorkDir{
		Root: root,
		Hash: hash,
		cfg:  cfg,
	}

	// Write project.json
	project := types.Project{
		AudioPath:  cfg.AudioPath,
		ImagesDir:  cfg.ImagesDir,
		OutputDir:  cfg.OutputDir,
		AudioHash:  audioHash,
		ImagesHash: imagesHash,
		Settings: map[string]string{
			"min_shot_sec":  fmt.Sprintf("%.1f", cfg.MinShotSec),
			"blur_strength": fmt.Sprintf("%d", cfg.BlurStrength),
			"whisper_model": cfg.WhisperModel,
		},
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}

	if err := wd.WriteJSON("project.json", project); err != nil {
		return nil, fmt.Errorf("failed to write project: %w", err)
	}

	return wd, nil
}

// Path returns the full path to a file in the work directory
func (w *WorkDir) Path(name string) string {
	return filepath.Join(w.Root, name)
}

// Exists checks if a file exists in the work directory
func (w *WorkDir) Exists(name string) bool {
	_, err := os.Stat(w.Path(name))
	return err == nil
}

// WriteJSON writes a JSON file to the work directory
func (w *WorkDir) WriteJSON(name string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(w.Path(name), data, 0644)
}

// ReadJSON reads a JSON file from the work directory
func (w *WorkDir) ReadJSON(name string, v any) error {
	data, err := os.ReadFile(w.Path(name))
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

func hashDirectory(dir string) (string, error) {
	var files []string

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && isImageFile(path) {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return "", err
	}

	sort.Strings(files)

	h := sha256.New()
	for _, f := range files {
		fileHash, err := hashFile(f)
		if err != nil {
			return "", err
		}
		h.Write([]byte(f + ":" + fileHash + "\n"))
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

func isImageFile(path string) bool {
	ext := filepath.Ext(path)
	switch ext {
	case ".jpg", ".jpeg", ".png", ".webp", ".bmp":
		return true
	default:
		return false
	}
}

// NewForImages creates a work directory for image-only operations
func NewForImages(cfg *config.Config) (*WorkDir, error) {
	imagesHash, err := hashDirectory(cfg.ImagesDir)
	if err != nil {
		return nil, fmt.Errorf("failed to hash images: %w", err)
	}

	combined := fmt.Sprintf("images:%s:%s", imagesHash[:16], cfg.VisionModel)
	h := sha256.Sum256([]byte(combined))
	hash := hex.EncodeToString(h[:])[:16]

	root := filepath.Join(cfg.WorkDir, hash)
	if err := os.MkdirAll(root, 0755); err != nil {
		return nil, fmt.Errorf("failed to create work directory: %w", err)
	}

	for _, sub := range []string{"images", "text", "render"} {
		if err := os.MkdirAll(filepath.Join(root, sub), 0755); err != nil {
			return nil, fmt.Errorf("failed to make work directory: %w", err)
		}
	}

	return &WorkDir{
		Root: root,
		Hash: hash,
		cfg:  cfg,
	}, nil
}
