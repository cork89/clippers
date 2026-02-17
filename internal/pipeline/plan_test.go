// ./internal/pipeline/plan_test.go
package pipeline

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cork89/clippers/internal/config"
	"github.com/cork89/clippers/internal/database"
	"github.com/cork89/clippers/internal/types"
)

func TestDetectDefaultImage(t *testing.T) {
	tmpDir := t.TempDir()
	if value := DetectDefaultImage(tmpDir); value != "" {
		t.Fatalf("expected empty result, got %q", value)
	}

	defaultPath := filepath.Join(tmpDir, "Default.jpg")
	if err := writeFile(defaultPath, "x"); err != nil {
		t.Fatalf("write default: %v", err)
	}

	value := DetectDefaultImage(tmpDir)
	if !strings.EqualFold(filepath.Base(value), filepath.Base(defaultPath)) {
		t.Fatalf("expected %q (case-insensitive), got %q", defaultPath, value)
	}
}

func TestBuildCatalogSummaryWithTitle(t *testing.T) {
	catalog := &database.ImageCatalog{
		Images: []database.ImageCaption{
			{ID: "default.png", Caption: "Default", Tags: []string{"default"}},
			{ID: "scene.jpg", Caption: "A city street", Tags: []string{"city", "night"}},
		},
	}

	summary := buildCatalogSummaryWithTitle(catalog, "My Title", true)
	if !strings.Contains(summary, "VIDEO TITLE") {
		t.Fatalf("expected title section, got: %s", summary)
	}
	if strings.Contains(summary, "default.png") && !strings.Contains(summary, "SPECIAL: default.png") {
		t.Fatalf("default should only appear in special section")
	}
	if !strings.Contains(summary, "scene.jpg") {
		t.Fatalf("expected image entry")
	}
}

func TestParseSelectionResponse_ClampAndErrors(t *testing.T) {
	jsonHigh := "{\"chosen_image_id\":\"a.jpg\",\"backup_image_id\":\"b.jpg\",\"confidence\":1.5,\"reason\":\"ok\",\"matches_title\":true,\"matches_segment\":false}"
	result, err := parseSelectionResponse(jsonHigh)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Confidence != 1 {
		t.Fatalf("expected confidence clamped to 1, got %.2f", result.Confidence)
	}

	jsonLow := "{\"chosen_image_id\":\"a.jpg\",\"backup_image_id\":\"b.jpg\",\"confidence\":-0.5,\"reason\":\"ok\",\"matches_title\":true,\"matches_segment\":false}"
	result, err = parseSelectionResponse(jsonLow)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Confidence != 0 {
		t.Fatalf("expected confidence clamped to 0, got %.2f", result.Confidence)
	}

	_, err = parseSelectionResponse("{\"chosen_image_id\":\"\"}")
	if err == nil {
		t.Fatal("expected error for empty chosen_image_id")
	}
}

func TestFindImagePath(t *testing.T) {
	catalog := &database.ImageCatalog{
		Images: []database.ImageCaption{
			{ID: "a.jpg", Path: "/tmp/a.jpg"},
			{ID: "b.jpg", Path: "/tmp/b.jpg"},
		},
	}

	value := findImagePath(catalog, "b.jpg")
	if value != "/tmp/b.jpg" {
		t.Fatalf("unexpected image path: %q", value)
	}
}

func TestSmoothTimeline_SmoothsLowConfidenceMiddle(t *testing.T) {
	entries := []types.TimelineEntry{
		{ImageID: "a.jpg", Image: "/tmp/a.jpg", Confidence: 0.9},
		{ImageID: "b.jpg", Image: "/tmp/b.jpg", Confidence: 0.2},
		{ImageID: "a.jpg", Image: "/tmp/a.jpg", Confidence: 0.9},
	}

	updated := smoothTimeline(entries, &database.ImageCatalog{}, "")
	if updated[1].ImageID != "a.jpg" {
		t.Fatalf("expected middle to be smoothed to a.jpg, got %q", updated[1].ImageID)
	}
}

func TestSmoothTimeline_DoesNotSmoothDefaultLowConfidence(t *testing.T) {
	entries := []types.TimelineEntry{
		{ImageID: "a.jpg", Image: "/tmp/a.jpg", Confidence: 0.9},
		{ImageID: "default.png", Image: "/tmp/default.png", Confidence: 0.2},
		{ImageID: "a.jpg", Image: "/tmp/a.jpg", Confidence: 0.9},
	}

	updated := smoothTimeline(entries, &database.ImageCatalog{}, "/tmp/default.png")
	if updated[1].ImageID != "default.png" {
		t.Fatalf("expected middle to remain default, got %q", updated[1].ImageID)
	}
}

func TestBuildSelectionPromptWithTitle(t *testing.T) {
	cfg := &config.Config{Title: "Demo Title"}
	window := database.TextWindowData{Start: 0, End: 5, Text: "hello"}

	prompt := buildSelectionPromptWithTitle(cfg, window, "CATALOG", "prev.jpg", true)
	if !strings.Contains(prompt, "Demo Title") {
		t.Fatalf("expected title in prompt")
	}
	if !strings.Contains(prompt, "prev.jpg") {
		t.Fatalf("expected previous image in prompt")
	}
	if !strings.Contains(prompt, "default.png") {
		t.Fatalf("expected default guidance in prompt")
	}
}

func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0644)
}
