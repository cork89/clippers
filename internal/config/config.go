// ./internal/config/config.go
package config

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

// ValidShaders returns all available shader options
func ValidShaders() []ShaderType {
	return []ShaderType{
		ShaderNone,
		ShaderWaveDisplace,
		ShaderEdgeGlow,
		ShaderLiquidFlow,
		ShaderPixelMelt,
		ShaderRetro,
		ShaderVoronoi,
	}
}

// IsValidShader checks if a shader name is valid
func IsValidShader(s string) bool {
	for _, shader := range ValidShaders() {
		if string(shader) == s {
			return true
		}
	}
	return false
}

// Config holds all configuration for a run
type Config struct {
	// Input paths
	AudioPath    string
	ImagesDir    string
	OutputDir    string
	DefaultImage string // Path to default.png if found

	// Content
	Title string // Video title for context

	// Working directory
	WorkDir string

	// Render settings
	Aspects      []string
	MinShotSec   float64
	MaxWords     int
	BlurStrength int
	FPS          int
	Shader       ShaderType // Shader effect to apply
	ShadersDir   string     // Directory containing shader files

	// Subtitle settings
	FontSize       int
	SubtitleMargin int

	// Planning settings
	DefaultImageWeight float64 // Confidence threshold below which default is used
	TitleWeight        string  // "high", "medium", "low"

	// Whisper settings
	WhisperModel string

	// Ollama settings
	OllamaHost  string
	VisionModel string
	SelectModel string

	// Flags
	Force bool
}

// DefaultConfig returns a config with default values
func DefaultConfig() *Config {
	return &Config{
		WorkDir:            ".work",
		Aspects:            []string{"1x1", "16x9", "9x16"},
		MinShotSec:         5,
		MaxWords:           5,
		BlurStrength:       20,
		FPS:                24,
		Shader:             ShaderNone,
		ShadersDir:         "shaders",
		FontSize:           60,
		SubtitleMargin:     20,
		DefaultImageWeight: 0.5,
		TitleWeight:        "high",
		WhisperModel:       "medium.en",
		OllamaHost:         "http://localhost:11434",
		VisionModel:        "gemma3:4b-it-qat",
		SelectModel:        "gemma3:4b-it-qat",
		Force:              false,
	}
}

// AspectConfig holds dimensions and subtitle settings for an aspect ratio
type AspectConfig struct {
	Width        int
	Height       int
	FontSize     int
	MarginV      int
	MaxLineChars int
}

// GetAspectConfig returns optimized settings for each aspect ratio
func GetAspectConfig(aspect string, baseFontSize int) AspectConfig {
	switch aspect {
	case "1x1":
		return AspectConfig{
			Width:        1080,
			Height:       1080,
			FontSize:     baseFontSize,
			MarginV:      40,
			MaxLineChars: 40,
		}
	case "16x9":
		return AspectConfig{
			Width:        1920,
			Height:       1080,
			FontSize:     baseFontSize + 4,
			MarginV:      30,
			MaxLineChars: 50,
		}
	case "9x16":
		return AspectConfig{
			Width:        1080,
			Height:       1920,
			FontSize:     baseFontSize - 2,
			MarginV:      80,
			MaxLineChars: 35,
		}
	default:
		return AspectConfig{
			Width:        1080,
			Height:       1080,
			FontSize:     baseFontSize,
			MarginV:      40,
			MaxLineChars: 40,
		}
	}
}
