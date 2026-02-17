// ./internal/pipeline/render_test.go
package pipeline

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cork89/clippers/internal/types"
)

func TestWriteConcatFile(t *testing.T) {
	tmpDir := t.TempDir()
	concatPath := filepath.Join(tmpDir, "concat.txt")

	entries := []types.TimelineEntry{
		{Start: 0, End: 2, Image: "/tmp/a.jpg"},
		{Start: 2, End: 5.5, Image: "/tmp/quote'file.jpg"},
	}

	timeline := &types.Timeline{Entries: entries}
	if err := writeConcatFile(concatPath, timeline); err != nil {
		t.Fatalf("writeConcatFile error: %v", err)
	}

	data, err := os.ReadFile(concatPath)
	if err != nil {
		t.Fatalf("read concat: %v", err)
	}

	text := string(data)
	if !strings.Contains(text, "duration 2.000") || !strings.Contains(text, "duration 3.500") {
		t.Fatalf("expected duration lines, got:\n%s", text)
	}
	if strings.Count(text, "file '") != 3 {
		t.Fatalf("expected 3 file lines (including repeat), got:\n%s", text)
	}
}
