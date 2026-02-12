// ./internal/pipeline/llm_provider.go
package pipeline

import (
	"os"

	"github.com/cork89/clippers/internal/config"
	"github.com/cork89/clippers/internal/llm"
)

func NewLLMProvider(cfg *config.Config) llm.Provider {
	switch cfg.LLMProvider {
	case config.LLMProviderOpenRouter:
		apiKey := os.Getenv("OPENROUTER_API_KEY")
		return llm.NewOpenRouterClient(apiKey)
	default:
		return llm.NewOllamaClient(cfg.OllamaHost)
	}
}
