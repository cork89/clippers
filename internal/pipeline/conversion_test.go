// ./internal/pipeline/conversion_test.go
package pipeline

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReplaceExt(t *testing.T) {
	value := replaceExt("file.srt", ".ass")
	if value != "file.ass" {
		t.Fatalf("unexpected replaceExt result: %q", value)
	}
}

func TestConvertSRTtoASSGo_GeneratesASS(t *testing.T) {
	tmpDir := t.TempDir()
	srtPath := filepath.Join(tmpDir, "subtitles.srt")
	assPath := filepath.Join(tmpDir, "subtitles.ass")

	content := "1\n00:00:00,000 --> 00:00:01,250\nHello {world}\n\n2\n00:00:01,250 --> 00:00:02,000\nLine 2\n"
	if err := os.WriteFile(srtPath, []byte(content), 0644); err != nil {
		t.Fatalf("write srt: %v", err)
	}

	if err := convertSRTtoASSGo(srtPath, assPath); err != nil {
		t.Fatalf("convertSRTtoASSGo error: %v", err)
	}

	data, err := os.ReadFile(assPath)
	if err != nil {
		t.Fatalf("read ass: %v", err)
	}
	text := string(data)

	required := []string{
		"[Script Info]",
		"[V4+ Styles]",
		"[Events]",
		"Format: Layer,Start,End,Style,Name,MarginL,MarginR,MarginV,Effect,Text",
		"Dialogue: 0,0:00:00.00,0:00:01.25,Default,,0,0,0,,Hello \\{world\\}",
		"Dialogue: 0,0:00:01.25,0:00:02.00,Default,,0,0,0,,Line 2",
	}

	for _, want := range required {
		if !strings.Contains(text, want) {
			t.Fatalf("expected ASS content to contain %q", want)
		}
	}
}
