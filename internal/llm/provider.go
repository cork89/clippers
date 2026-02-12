// ./internal/llm/provider.go
package llm

type Provider interface {
	GenerateWithImage(model, prompt, imagePath string, jsonMode bool) (string, error)
	GenerateText(model, prompt string, jsonMode bool) (string, error)
	Ping() error
}
