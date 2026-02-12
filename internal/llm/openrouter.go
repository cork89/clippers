// ./internal/llm/openrouter.go
package llm

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"strings"

	openrouter "github.com/revrost/go-openrouter"
)

type OpenRouterClient struct {
	client *openrouter.Client
	apiKey string
}

func NewOpenRouterClient(apiKey string) *OpenRouterClient {
	client := openrouter.NewClient(apiKey)
	return &OpenRouterClient{
		client: client,
		apiKey: apiKey,
	}
}

func (c *OpenRouterClient) GenerateWithImage(model, prompt, imagePath string, jsonMode bool) (string, error) {
	imageData, err := os.ReadFile(imagePath)
	if err != nil {
		return "", fmt.Errorf("failed to read image: %w", err)
	}

	mimeType := detectMimeType(imagePath)
	base64Data := base64.StdEncoding.EncodeToString(imageData)
	dataURL := fmt.Sprintf("data:%s;base64,%s", mimeType, base64Data)

	ctx := context.Background()

	messages := []openrouter.ChatCompletionMessage{
		{
			Role: openrouter.ChatMessageRoleUser,
			Content: openrouter.Content{
				Multi: []openrouter.ChatMessagePart{
					{
						Type: openrouter.ChatMessagePartTypeText,
						Text: prompt,
					},
					{
						Type: openrouter.ChatMessagePartTypeImageURL,
						ImageURL: &openrouter.ChatMessageImageURL{
							URL: dataURL,
						},
					},
				},
			},
		},
	}

	req := openrouter.ChatCompletionRequest{
		Model:    model,
		Messages: messages,
	}

	if jsonMode {
		req.ResponseFormat = &openrouter.ChatCompletionResponseFormat{
			Type: openrouter.ChatCompletionResponseFormatTypeJSONObject,
		}
	}

	resp, err := c.client.CreateChatCompletion(ctx, req)
	if err != nil {
		return "", fmt.Errorf("openrouter request failed: %w", err)
	}

	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("no response choices returned")
	}

	return resp.Choices[0].Message.Content.Text, nil
}

func (c *OpenRouterClient) GenerateText(model, prompt string, jsonMode bool) (string, error) {
	ctx := context.Background()

	messages := []openrouter.ChatCompletionMessage{
		openrouter.UserMessage(prompt),
	}

	req := openrouter.ChatCompletionRequest{
		Model:    model,
		Messages: messages,
	}

	if jsonMode {
		req.ResponseFormat = &openrouter.ChatCompletionResponseFormat{
			Type: openrouter.ChatCompletionResponseFormatTypeJSONObject,
		}
	}

	resp, err := c.client.CreateChatCompletion(ctx, req)
	if err != nil {
		return "", fmt.Errorf("openrouter request failed: %w", err)
	}

	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("no response choices returned")
	}

	return resp.Choices[0].Message.Content.Text, nil
}

func (c *OpenRouterClient) Ping() error {
	if c.apiKey == "" {
		return fmt.Errorf("OPENROUTER_API_KEY not set in .env file")
	}
	return nil
}

func detectMimeType(path string) string {
	ext := strings.ToLower(path)
	if strings.HasSuffix(ext, ".png") {
		return "image/png"
	}
	if strings.HasSuffix(ext, ".gif") {
		return "image/gif"
	}
	if strings.HasSuffix(ext, ".webp") {
		return "image/webp"
	}
	return "image/jpeg"
}
