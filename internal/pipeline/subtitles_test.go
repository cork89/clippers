// ./internal/pipeline/subtitles_test.go
package pipeline

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cork89/clippers/internal/config"
	"github.com/cork89/clippers/internal/types"
)

func TestQuantizeToFrameRate_DefaultFPS(t *testing.T) {
	value := quantizeToFrameRate(1.0, 0)
	if value != 1.0 {
		t.Fatalf("expected 1.0, got %.6f", value)
	}
}

func TestFormatSRTTime_Rounding(t *testing.T) {
	value := formatSRTTime(1.999)
	if value != "00:00:01,999" {
		t.Fatalf("unexpected format: %s", value)
	}
}

func TestRemoveTags(t *testing.T) {
	value := removeTags("{\\i1}Hello{\\i0} world")
	if value != "Hello world" {
		t.Fatalf("unexpected tag removal: %q", value)
	}
}

func TestRoundedRectPath(t *testing.T) {
	value := roundedRectPath(100, 50, 80)
	if !strings.HasPrefix(value, "m ") {
		t.Fatalf("expected path to start with move command, got %q", value)
	}
}

func TestAddRoundedBackground_InsertsStylesAndPlayRes(t *testing.T) {
	tmpDir := t.TempDir()
	inputPath := filepath.Join(tmpDir, "raw.ass")
	outputPath := filepath.Join(tmpDir, "out.ass")

	content := strings.Join([]string{
		"[Script Info]",
		"Title: Test",
		"",
		"[V4+ Styles]",
		"Format: Name, Fontname, Fontsize, PrimaryColour, SecondaryColour, OutlineColour, BackColour, Bold, Italic, Underline, StrikeOut, ScaleX, ScaleY, Spacing, Angle, BorderStyle, Outline, Shadow, Alignment, MarginL, MarginR, MarginV, Encoding",
		"Style: Default,Arial,20,&H00FFFFFF,&H000000FF,&H00000000,&H64000000,0,0,0,0,100,100,0,0,1,1,0,2,10,10,10,1",
		"",
		"[Events]",
		"Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text",
		"Dialogue: 0,0:00:01.00,0:00:02.00,Default,,0,0,0,,Hello",
		"",
	}, "\n")

	if err := os.WriteFile(inputPath, []byte(content), 0644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	aspectCfg := config.GetAspectConfig("1x1", 40)
	subAspect := types.SubtitleAspect{Aspect: "1x1", Path: outputPath}

	if err := addRoundedBackground(inputPath, subAspect, aspectCfg); err != nil {
		t.Fatalf("addRoundedBackground error: %v", err)
	}

	out, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}

	text := string(out)
	if !strings.Contains(text, "PlayResX:") || !strings.Contains(text, "PlayResY:") {
		t.Fatalf("expected PlayRes lines, got:\n%s", text)
	}
	if !strings.Contains(text, "Style: RoundedBox") || !strings.Contains(text, "Style: SubText") {
		t.Fatalf("expected injected styles, got:\n%s", text)
	}
	if !strings.Contains(text, "Dialogue: 0") || !strings.Contains(text, "Dialogue: 1") {
		t.Fatalf("expected background + text dialogues, got:\n%s", text)
	}
}
