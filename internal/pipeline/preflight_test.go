// ./internal/pipeline/preflight_test.go
package pipeline

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cork89/clippers/internal/config"
	"github.com/cork89/clippers/internal/workdir"
)

func TestListImages_ExtensionsAndDirs(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "a.jpg"), []byte("x"), 0644); err != nil {
		t.Fatalf("write a.jpg: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "b.JPEG"), []byte("x"), 0644); err != nil {
		t.Fatalf("write b.JPEG: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "c.png"), []byte("x"), 0644); err != nil {
		t.Fatalf("write c.png: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "d.txt"), []byte("x"), 0644); err != nil {
		t.Fatalf("write d.txt: %v", err)
	}
	if err := os.Mkdir(filepath.Join(tmpDir, "sub"), 0755); err != nil {
		t.Fatalf("mkdir sub: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "sub", "e.jpg"), []byte("x"), 0644); err != nil {
		t.Fatalf("write e.jpg: %v", err)
	}

	images, err := listImages(tmpDir)
	if err != nil {
		t.Fatalf("listImages error: %v", err)
	}
	if len(images) != 3 {
		t.Fatalf("expected 3 images, got %d", len(images))
	}
}

func TestNeedsLLM(t *testing.T) {
	cfg := &config.Config{Force: false}
	wd := &workdir.WorkDir{Root: t.TempDir()}

	if !needsLLM(cfg, wd) {
		t.Fatal("expected needsLLM when cache missing")
	}

	if err := os.MkdirAll(filepath.Join(wd.Root, "images"), 0755); err != nil {
		t.Fatalf("mkdir images: %v", err)
	}
	if err := os.WriteFile(filepath.Join(wd.Root, "images", "captions.json"), []byte("{}"), 0644); err != nil {
		t.Fatalf("write captions: %v", err)
	}
	if err := os.WriteFile(filepath.Join(wd.Root, "timeline.json"), []byte("{}"), 0644); err != nil {
		t.Fatalf("write timeline: %v", err)
	}

	if needsLLM(cfg, wd) {
		t.Fatal("expected needsLLM false when cache exists")
	}

	cfg.Force = true
	if !needsLLM(cfg, wd) {
		t.Fatal("expected needsLLM true when forced")
	}
}
