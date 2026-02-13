// ./internal/config/config.go
package config

import (
	"os"
	"strconv"
	"strings"
)

type LLMProvider string

const (
	LLMProviderOllama     LLMProvider = "ollama"
	LLMProviderOpenRouter LLMProvider = "openrouter"
)

const (
	DefaultOpenRouterVisionModel = "openai/gpt-5-nano"
	DefaultOpenRouterSelectModel = "openai/gpt-5-nano"
)

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

func IsValidLLMProvider(s string) bool {
	return s == string(LLMProviderOllama) || s == string(LLMProviderOpenRouter)
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

	// LLM Provider settings
	LLMProvider LLMProvider

	// Ollama settings
	OllamaHost  string
	VisionModel string
	SelectModel string

	// Flags
	Force bool
}

func getEnvString(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func getEnvInt(key string, defaultVal int) int {
	if val := os.Getenv(key); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
	}
	return defaultVal
}

func getEnvFloat(key string, defaultVal float64) float64 {
	if val := os.Getenv(key); val != "" {
		if f, err := strconv.ParseFloat(val, 64); err == nil {
			return f
		}
	}
	return defaultVal
}

func getEnvBool(key string, defaultVal bool) bool {
	if val := os.Getenv(key); val != "" {
		if b, err := strconv.ParseBool(val); err == nil {
			return b
		}
	}
	return defaultVal
}

func getEnvStringSlice(key string, defaultVal []string) []string {
	if val := os.Getenv(key); val != "" {
		return strings.Split(val, ",")
	}
	return defaultVal
}

// DefaultConfig returns a config with default values
func DefaultConfig() *Config {
	return &Config{
		WorkDir:            getEnvString("CLIPPERS_WORK_DIR", ".work"),
		Aspects:            getEnvStringSlice("CLIPPERS_ASPECTS", []string{"1x1", "16x9", "9x16"}),
		MinShotSec:         getEnvFloat("CLIPPERS_MIN_SHOT_SEC", 5),
		MaxWords:           getEnvInt("CLIPPERS_MAX_WORDS", 5),
		BlurStrength:       getEnvInt("CLIPPERS_BLUR_STRENGTH", 20),
		FPS:                getEnvInt("CLIPPERS_FPS", 24),
		Shader:             ShaderType(getEnvString("CLIPPERS_SHADER", string(ShaderNone))),
		ShadersDir:         getEnvString("CLIPPERS_SHADERS_DIR", "shaders"),
		FontSize:           getEnvInt("CLIPPERS_FONT_SIZE", 60),
		SubtitleMargin:     getEnvInt("CLIPPERS_SUBTITLE_MARGIN", 20),
		DefaultImageWeight: getEnvFloat("CLIPPERS_DEFAULT_IMAGE_WEIGHT", 0.5),
		TitleWeight:        getEnvString("CLIPPERS_TITLE_WEIGHT", "high"),
		WhisperModel:       getEnvString("CLIPPERS_WHISPER_MODEL", "distil-medium.en"),
		LLMProvider:        LLMProvider(getEnvString("CLIPPERS_LLM_PROVIDER", string(LLMProviderOllama))),
		OllamaHost:         getEnvString("CLIPPERS_OLLAMA_HOST", "http://localhost:11434"),
		VisionModel:        getEnvString("CLIPPERS_VISION_MODEL", "gemma3:4b-it-qat"),
		SelectModel:        getEnvString("CLIPPERS_SELECT_MODEL", "gemma3:4b-it-qat"),
		Force:              getEnvBool("CLIPPERS_FORCE", false),
	}
}

func (c *Config) ReloadFromEnv() {
	c.LLMProvider = LLMProvider(getEnvString("CLIPPERS_LLM_PROVIDER", string(c.LLMProvider)))
	c.OllamaHost = getEnvString("CLIPPERS_OLLAMA_HOST", c.OllamaHost)
	c.VisionModel = getEnvString("CLIPPERS_VISION_MODEL", c.VisionModel)
	c.SelectModel = getEnvString("CLIPPERS_SELECT_MODEL", c.SelectModel)
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
