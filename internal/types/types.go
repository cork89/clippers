package types

import (
	"embed"
	"fmt"
)

//go:embed shaders/*.glsl
var shadersFS embed.FS

// ShaderType represents available shader effects
type ShaderType string

const (
	ShaderNone         ShaderType = "none"
	ShaderWaveDisplace ShaderType = "wave_displace"
	ShaderEdgeGlow     ShaderType = "edge_glow"
	ShaderLiquidFlow   ShaderType = "liquid_flow"
	ShaderPixelMelt    ShaderType = "pixel_melt"
	ShaderRetro        ShaderType = "retro"
	ShaderVoronoi      ShaderType = "voronoi"
)

type ShaderOption struct {
	Value ShaderType
	Label string
	Color string
}

func ShaderOptions() []ShaderOption {
	return []ShaderOption{
		{Value: ShaderNone, Label: "None", Color: "#3a3a3a"},
		{Value: ShaderWaveDisplace, Label: "Wave", Color: "#6366f1"},
		{Value: ShaderEdgeGlow, Label: "Edge", Color: "#ec4899"},
		{Value: ShaderLiquidFlow, Label: "Liquid", Color: "#14b8a6"},
		{Value: ShaderPixelMelt, Label: "Pixel", Color: "#f59e0b"},
		{Value: ShaderRetro, Label: "Retro", Color: "#8b5cf6"},
		{Value: ShaderVoronoi, Label: "Voronoi", Color: "#10b981"},
	}
}

func ListShaders() []ShaderType {
	entries, _ := shadersFS.ReadDir("shaders")
	var shaders []ShaderType
	for _, e := range entries {
		if !e.IsDir() && hasSuffix(e.Name(), "_browser.glsl") {`
			name := stripSuffix(e.Name(), "_browser.glsl")
			shaders = append(shaders, ShaderType(name))
		}
	}
	return shaders
}

func GetShader(name ShaderType) (string, error) {
	data, err := shadersFS.ReadFile(fmt.Sprintf("shaders/%s_browser.glsl", name))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func GetVertexShader() (string, error) {
	data, err := shadersFS.ReadFile("shaders/vertex.glsl")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func hasSuffix(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}

func stripSuffix(s, suffix string) string {
	return s[:len(s)-len(suffix)]
}

// Segment represents a transcribed segment from whisper
type Segment struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
	Text  string  `json:"text"`
}

// Transcript holds the full transcription result
type Transcript struct {
	Language    string    `json:"language"`
	DurationSec float64   `json:"duration_sec"`
	Segments    []Segment `json:"segments"`
}

// TimelineEntry represents a single shot in the video timeline
type TimelineEntry struct {
	Start      float64 `json:"start"`
	End        float64 `json:"end"`
	ImageID    string  `json:"image_id"`
	Image      string  `json:"image_path"`
	Confidence float64 `json:"confidence,omitempty"`
	Reason     string  `json:"reason,omitempty"`
}

// ProjectSettings represents project-level settings stored in the database
type ProjectSettings struct {
	ProjectID          string  `json:"project_id"`
	Shader             string  `json:"shader"`
	FPS                int     `json:"fps"`
	Aspects            string  `json:"aspects"`
	FontSize           int     `json:"font_size"`
	SubtitleMargin     int     `json:"subtitle_margin"`
	MinShotSec         float64 `json:"min_shot_sec"`
	MaxWords           int     `json:"max_words"`
	DefaultImageWeight float64 `json:"default_image_weight"`
	TitleWeight        string  `json:"title_weight"`
	BlurStrength       int     `json:"blur_strength"`
	WhisperModel       string  `json:"whisper_model"`
	VisionModel        string  `json:"vision_model"`
	SelectModel        string  `json:"select_model"`
}

// Timeline is the full video timeline
type Timeline struct {
	Entries []TimelineEntry `json:"entries"`
}

// Project stores the project metadata
type Project struct {
	AudioPath  string            `json:"audio_path"`
	ImagesDir  string            `json:"images_dir"`
	OutputDir  string            `json:"output_dir"`
	AudioHash  string            `json:"audio_hash"`
	ImagesHash string            `json:"images_hash"`
	Settings   map[string]string `json:"settings"`
	CreatedAt  string            `json:"created_at"`
}

// SRTCue represents a single subtitle cue
type SRTCue struct {
	Index int
	Start float64
	End   float64
	Text  string
}

type SubtitleAspect struct {
	Aspect string
	Path   string
}
