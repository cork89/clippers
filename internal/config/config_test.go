// ./internal/config/config_test.go
package config

import (
	"os"
	"testing"

	"github.com/cork89/clippers/internal/types"
)

func TestGetEnvHelpers(t *testing.T) {
	t.Setenv("CLIPPERS_TEST_STR", "value")
	if got := getEnvString("CLIPPERS_TEST_STR", "default"); got != "value" {
		t.Fatalf("getEnvString: expected value, got %q", got)
	}
	if got := getEnvString("CLIPPERS_TEST_STR_MISSING", "default"); got != "default" {
		t.Fatalf("getEnvString: expected default, got %q", got)
	}

	t.Setenv("CLIPPERS_TEST_INT", "42")
	if got := getEnvInt("CLIPPERS_TEST_INT", 7); got != 42 {
		t.Fatalf("getEnvInt: expected 42, got %d", got)
	}
	if got := getEnvInt("CLIPPERS_TEST_INT_BAD", 7); got != 7 {
		t.Fatalf("getEnvInt: expected default on bad value, got %d", got)
	}

	t.Setenv("CLIPPERS_TEST_FLOAT", "3.5")
	if got := getEnvFloat("CLIPPERS_TEST_FLOAT", 1.2); got != 3.5 {
		t.Fatalf("getEnvFloat: expected 3.5, got %.2f", got)
	}
	if got := getEnvFloat("CLIPPERS_TEST_FLOAT_BAD", 1.2); got != 1.2 {
		t.Fatalf("getEnvFloat: expected default on bad value, got %.2f", got)
	}

	t.Setenv("CLIPPERS_TEST_BOOL", "true")
	if got := getEnvBool("CLIPPERS_TEST_BOOL", false); got != true {
		t.Fatalf("getEnvBool: expected true, got %v", got)
	}
	if got := getEnvBool("CLIPPERS_TEST_BOOL_BAD", true); got != true {
		t.Fatalf("getEnvBool: expected default on bad value, got %v", got)
	}

	t.Setenv("CLIPPERS_TEST_LIST", "a,b,c")
	list := getEnvStringSlice("CLIPPERS_TEST_LIST", []string{"x"})
	if len(list) != 3 || list[0] != "a" || list[2] != "c" {
		t.Fatalf("getEnvStringSlice: unexpected list %v", list)
	}
}

func TestDefaultConfig_EnvOverrides(t *testing.T) {
	t.Setenv("CLIPPERS_WORK_DIR", ".workdir")
	t.Setenv("CLIPPERS_ASPECTS", "1x1,16x9")
	t.Setenv("CLIPPERS_MIN_SHOT_SEC", "7.5")
	t.Setenv("CLIPPERS_MAX_WORDS", "9")
	t.Setenv("CLIPPERS_BLUR_STRENGTH", "12")
	t.Setenv("CLIPPERS_FPS", "30")
	t.Setenv("CLIPPERS_SHADER", string(types.ShaderRetro))
	t.Setenv("CLIPPERS_SHADERS_DIR", "shaders-custom")
	t.Setenv("CLIPPERS_FONT_SIZE", "48")
	t.Setenv("CLIPPERS_SUBTITLE_MARGIN", "25")
	t.Setenv("CLIPPERS_DEFAULT_IMAGE_WEIGHT", "0.8")
	t.Setenv("CLIPPERS_TITLE_WEIGHT", "medium")
	t.Setenv("CLIPPERS_WHISPER_MODEL", "small")
	t.Setenv("CLIPPERS_LLM_PROVIDER", string(LLMProviderOpenRouter))
	t.Setenv("CLIPPERS_OLLAMA_HOST", "http://example.com")
	t.Setenv("CLIPPERS_VISION_MODEL", "vision-model")
	t.Setenv("CLIPPERS_SELECT_MODEL", "select-model")
	t.Setenv("CLIPPERS_FORCE", "true")
	t.Setenv("CLIPPERS_USE_GO_ASS_CONVERSION", "true")

	cfg := DefaultConfig()
	if cfg.WorkDir != ".workdir" {
		t.Fatalf("WorkDir override failed: %q", cfg.WorkDir)
	}
	if len(cfg.Aspects) != 2 || cfg.Aspects[0] != "1x1" || cfg.Aspects[1] != "16x9" {
		t.Fatalf("Aspects override failed: %v", cfg.Aspects)
	}
	if cfg.MinShotSec != 7.5 || cfg.MaxWords != 9 || cfg.BlurStrength != 12 {
		t.Fatalf("numeric overrides failed: %.1f %d %d", cfg.MinShotSec, cfg.MaxWords, cfg.BlurStrength)
	}
	if cfg.FPS != 30 || cfg.Shader != types.ShaderRetro || cfg.ShadersDir != "shaders-custom" {
		t.Fatalf("render overrides failed: %d %s %s", cfg.FPS, cfg.Shader, cfg.ShadersDir)
	}
	if cfg.FontSize != 48 || cfg.SubtitleMargin != 25 {
		t.Fatalf("subtitle overrides failed: %d %d", cfg.FontSize, cfg.SubtitleMargin)
	}
	if cfg.DefaultImageWeight != 0.8 || cfg.TitleWeight != "medium" {
		t.Fatalf("planning overrides failed: %.2f %s", cfg.DefaultImageWeight, cfg.TitleWeight)
	}
	if cfg.WhisperModel != "small" {
		t.Fatalf("whisper override failed: %s", cfg.WhisperModel)
	}
	if cfg.LLMProvider != LLMProviderOpenRouter || cfg.OllamaHost != "http://example.com" {
		t.Fatalf("provider overrides failed: %s %s", cfg.LLMProvider, cfg.OllamaHost)
	}
	if cfg.VisionModel != "vision-model" || cfg.SelectModel != "select-model" {
		t.Fatalf("model overrides failed: %s %s", cfg.VisionModel, cfg.SelectModel)
	}
	if cfg.Force != true {
		t.Fatalf("force override failed")
	}
	if cfg.UseGoASSConversion != true {
		t.Fatalf("UseGoASSConversion override failed")
	}
}

func TestReloadFromEnv(t *testing.T) {
	cfg := &Config{
		LLMProvider:        LLMProviderOllama,
		OllamaHost:         "http://localhost:11434",
		VisionModel:        "vision-old",
		SelectModel:        "select-old",
		UseGoASSConversion: false,
	}

	t.Setenv("CLIPPERS_LLM_PROVIDER", string(LLMProviderOpenRouter))
	t.Setenv("CLIPPERS_OLLAMA_HOST", "http://changed")
	t.Setenv("CLIPPERS_VISION_MODEL", "vision-new")
	t.Setenv("CLIPPERS_SELECT_MODEL", "select-new")
	t.Setenv("CLIPPERS_USE_GO_ASS_CONVERSION", "true")

	cfg.ReloadFromEnv()

	if cfg.LLMProvider != LLMProviderOpenRouter {
		t.Fatalf("expected LLMProvider updated, got %s", cfg.LLMProvider)
	}
	if cfg.OllamaHost != "http://changed" {
		t.Fatalf("expected OllamaHost updated, got %s", cfg.OllamaHost)
	}
	if cfg.VisionModel != "vision-new" || cfg.SelectModel != "select-new" {
		t.Fatalf("model reload failed: %s %s", cfg.VisionModel, cfg.SelectModel)
	}
	if cfg.UseGoASSConversion != true {
		t.Fatalf("UseGoASSConversion reload failed")
	}
}

func TestIsValidLLMProvider(t *testing.T) {
	if !IsValidLLMProvider(string(LLMProviderOllama)) {
		t.Fatalf("expected ollama valid")
	}
	if !IsValidLLMProvider(string(LLMProviderOpenRouter)) {
		t.Fatalf("expected openrouter valid")
	}
	if IsValidLLMProvider("nope") {
		t.Fatalf("expected invalid provider")
	}
}

func TestValidShadersAndCheck(t *testing.T) {
	shaders := ValidShaders()
	if len(shaders) == 0 {
		t.Fatalf("expected shaders list")
	}
	if !IsValidShader(string(types.ShaderNone)) {
		t.Fatalf("expected ShaderNone valid")
	}
	if IsValidShader("nope") {
		t.Fatalf("expected invalid shader")
	}
}

func TestGetAspectConfig(t *testing.T) {
	cfg := GetAspectConfig("16x9", 40)
	if cfg.Width != 1920 || cfg.Height != 1080 || cfg.FontSize != 44 {
		t.Fatalf("unexpected 16x9 config: %+v", cfg)
	}

	cfg = GetAspectConfig("9x16", 40)
	if cfg.Width != 1080 || cfg.Height != 1920 || cfg.FontSize != 38 {
		t.Fatalf("unexpected 9x16 config: %+v", cfg)
	}

	cfg = GetAspectConfig("unknown", 40)
	if cfg.Width != 1080 || cfg.Height != 1080 {
		t.Fatalf("unexpected fallback config: %+v", cfg)
	}
}

func TestGetEnvIntBadValue(t *testing.T) {
	_ = os.Setenv("CLIPPERS_TEST_INT_BAD", "notanint")
	if got := getEnvInt("CLIPPERS_TEST_INT_BAD", 7); got != 7 {
		t.Fatalf("expected default for bad int, got %d", got)
	}
}
