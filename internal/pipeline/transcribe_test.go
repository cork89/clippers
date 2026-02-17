// ./internal/pipeline/transcribe_test.go
package pipeline

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseSRT_MultiLineAndSpacing(t *testing.T) {
	tmpDir := t.TempDir()
	srtPath := filepath.Join(tmpDir, "audio.srt")

	content := "1\n00:00:00,000 --> 00:00:02,500\nHello\nworld\n\n2\n00:00:02,500 --> 00:00:04,000\nSecond line\n\n"
	if err := os.WriteFile(srtPath, []byte(content), 0644); err != nil {
		t.Fatalf("write srt: %v", err)
	}

	transcript, err := parseSRT(srtPath)
	if err != nil {
		t.Fatalf("parseSRT error: %v", err)
	}

	if len(transcript.Segments) != 2 {
		t.Fatalf("expected 2 segments, got %d", len(transcript.Segments))
	}

	first := transcript.Segments[0]
	if first.Text != "Hello world" {
		t.Fatalf("expected merged text, got %q", first.Text)
	}
	if first.Start != 0 || first.End != 2.5 {
		t.Fatalf("unexpected first timing: %.3f -> %.3f", first.Start, first.End)
	}

	second := transcript.Segments[1]
	if second.Text != "Second line" {
		t.Fatalf("unexpected second text: %q", second.Text)
	}
	if second.Start != 2.5 || second.End != 4.0 {
		t.Fatalf("unexpected second timing: %.3f -> %.3f", second.Start, second.End)
	}
}

func TestParseTimestamp(t *testing.T) {
	value := parseTimestamp("01", "02", "03", "400")
	if value != 3723.4 {
		t.Fatalf("expected 3723.4, got %.4f", value)
	}
}

func TestShouldRetryWhisperOnCPU(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   bool
	}{
		{
			name:   "cuda not compiled",
			output: "ValueError: This CTranslate2 package was not compiled with CUDA support",
			want:   true,
		},
		{
			name:   "missing cudnn",
			output: "failed to load shared library libcudnn_ops_infer.so",
			want:   true,
		},
		{
			name:   "generic transcription error",
			output: "FileNotFoundError: audio.wav",
			want:   false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := shouldRetryWhisperOnCPU(tc.output)
			if got != tc.want {
				t.Fatalf("shouldRetryWhisperOnCPU() = %v, want %v", got, tc.want)
			}
		})
	}
}
