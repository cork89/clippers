// ./internal/pipeline/caption_test.go
package pipeline

import (
	"strings"
	"testing"
)

func TestParseCaptionResponse_ValidJSONAndFences(t *testing.T) {
	input := "```json\n{\"caption\":\"A small dog runs\",\"tags\":[\"dog\",\"pet\"],\"style\":2,\"notes\":\"bright\"}\n```"

	caption, err := parseCaptionResponse(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if caption.Caption != "A small dog runs" {
		t.Fatalf("unexpected caption: %q", caption.Caption)
	}
	if len(caption.Tags) != 2 {
		t.Fatalf("expected tags, got %v", caption.Tags)
	}
}

func TestParseCaptionResponse_DefaultTags(t *testing.T) {
	input := "{\"caption\":\"A tree\",\"tags\":[],\"style\":1,\"notes\":\"\"}"

	caption, err := parseCaptionResponse(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(caption.Tags) != 1 || caption.Tags[0] != "image" {
		t.Fatalf("expected default tag, got %v", caption.Tags)
	}
}

func TestParseCaptionResponse_InvalidJSON(t *testing.T) {
	_, err := parseCaptionResponse("{not json}")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "invalid JSON") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTruncate(t *testing.T) {
	value := truncate("abcdef", 4)
	if value != "abcd..." {
		t.Fatalf("unexpected truncate result: %q", value)
	}

	value = truncate("abc", 4)
	if value != "abc" {
		t.Fatalf("unexpected truncate result: %q", value)
	}
}
