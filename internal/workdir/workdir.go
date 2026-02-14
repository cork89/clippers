package workdir

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/cork89/clippers/internal/config"
	"github.com/cork89/clippers/internal/database"
	"github.com/cork89/clippers/internal/types"
)

type WorkDir struct {
	Root string
	Hash string
	cfg  *config.Config
	db   *database.DB
}

func New(ctx context.Context, cfg *config.Config, db *database.DB) (*WorkDir, error) {
	audioHash, err := hashFile(cfg.AudioPath)
	if err != nil {
		return nil, fmt.Errorf("failed to hash audio: %w", err)
	}

	imagesHash, err := hashDirectory(cfg.ImagesDir)
	if err != nil {
		return nil, fmt.Errorf("failed to hash images: %w", err)
	}

	combined := fmt.Sprintf("%s:%s:%.1f:%d", audioHash[:16], imagesHash[:16], cfg.MinShotSec, cfg.BlurStrength)
	h := sha256.Sum256([]byte(combined))
	hash := hex.EncodeToString(h[:])[:16]

	root := filepath.Join(cfg.WorkDir, hash)
	if err := os.MkdirAll(root, 0755); err != nil {
		return nil, fmt.Errorf("failed to create work directory: %w", err)
	}

	wd := &WorkDir{
		Root: root,
		Hash: hash,
		cfg:  cfg,
		db:   db,
	}

	settings := map[string]string{
		"min_shot_sec":  fmt.Sprintf("%.1f", cfg.MinShotSec),
		"blur_strength": fmt.Sprintf("%d", cfg.BlurStrength),
		"whisper_model": cfg.WhisperModel,
	}

	if err := db.CreateProject(ctx, hash, cfg.AudioPath, cfg.ImagesDir, cfg.OutputDir, audioHash, imagesHash, settings); err != nil {
		return nil, fmt.Errorf("failed to create project in database: %w", err)
	}

	projectSettings := &types.ProjectSettings{
		ProjectID:          hash,
		Shader:             string(cfg.Shader),
		FPS:                cfg.FPS,
		Aspects:            strings.Join(cfg.Aspects, ","),
		FontSize:           cfg.FontSize,
		SubtitleMargin:     cfg.SubtitleMargin,
		MinShotSec:         cfg.MinShotSec,
		MaxWords:           cfg.MaxWords,
		DefaultImageWeight: cfg.DefaultImageWeight,
		TitleWeight:        cfg.TitleWeight,
		BlurStrength:       cfg.BlurStrength,
		WhisperModel:       cfg.WhisperModel,
		VisionModel:        cfg.VisionModel,
		SelectModel:        cfg.SelectModel,
	}

	if err := db.SaveProjectSettings(ctx, projectSettings); err != nil {
		return nil, fmt.Errorf("failed to save project settings: %w", err)
	}

	return wd, nil
}

func (w *WorkDir) Path(name string) string {
	return filepath.Join(w.Root, name)
}

func (w *WorkDir) Exists(name string) bool {
	_, err := os.Stat(w.Path(name))
	return err == nil
}

func (w *WorkDir) DB() *database.DB {
	return w.db
}

func (w *WorkDir) ProjectID() string {
	return w.Hash
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

func NewForImages(ctx context.Context, cfg *config.Config, db *database.DB) (*WorkDir, error) {
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

	return &WorkDir{
		Root: root,
		Hash: hash,
		cfg:  cfg,
		db:   db,
	}, nil
}
