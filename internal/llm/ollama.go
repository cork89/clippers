// ./internal/llm/ollama.go
package llm

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

type OllamaClient struct {
	host       string
	httpClient *http.Client
}

func NewOllamaClient(host string) *OllamaClient {
	return &OllamaClient{
		host: host,
		httpClient: &http.Client{
			Timeout: 5 * time.Minute,
		},
	}
}

type ollamaGenerateRequest struct {
	Model   string         `json:"model"`
	Prompt  string         `json:"prompt"`
	Images  []string       `json:"images,omitempty"`
	Stream  bool           `json:"stream"`
	Format  string         `json:"format,omitempty"`
	Options *ollamaOptions `json:"options,omitempty"`
}

type ollamaOptions struct {
	Temperature float64 `json:"temperature,omitempty"`
	Seed        int     `json:"seed,omitempty"`
}

type ollamaGenerateResponse struct {
	Model    string `json:"model"`
	Response string `json:"response"`
	Done     bool   `json:"done"`
	Error    string `json:"error,omitempty"`
}

func (c *OllamaClient) generate(req *ollamaGenerateRequest) (*ollamaGenerateResponse, error) {
	req.Stream = false

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	resp, err := c.httpClient.Post(
		c.host+"/api/generate",
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ollama returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var result ollamaGenerateResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if result.Error != "" {
		return nil, fmt.Errorf("ollama error: %s", result.Error)
	}

	return &result, nil
}

func (c *OllamaClient) GenerateWithImage(model, prompt, imagePath string, jsonMode bool) (string, error) {
	imageData, err := os.ReadFile(imagePath)
	if err != nil {
		return "", fmt.Errorf("failed to read image: %w", err)
	}

	encoded := base64.StdEncoding.EncodeToString(imageData)

	req := &ollamaGenerateRequest{
		Model:  model,
		Prompt: prompt,
		Images: []string{encoded},
		Options: &ollamaOptions{
			Temperature: 0,
		},
	}

	if jsonMode {
		req.Format = "json"
	}

	resp, err := c.generate(req)
	if err != nil {
		return "", err
	}

	return resp.Response, nil
}

func (c *OllamaClient) GenerateText(model, prompt string, jsonMode bool) (string, error) {
	req := &ollamaGenerateRequest{
		Model:  model,
		Prompt: prompt,
		Options: &ollamaOptions{
			Temperature: 0,
		},
	}

	if jsonMode {
		req.Format = "json"
	}

	resp, err := c.generate(req)
	if err != nil {
		return "", err
	}

	return resp.Response, nil
}

func (c *OllamaClient) Ping() error {
	resp, err := c.httpClient.Get(c.host + "/api/tags")
	if err != nil {
		return fmt.Errorf("cannot reach Ollama at %s: %w", c.host, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ollama returned status %d", resp.StatusCode)
	}

	return nil
}

func (c *OllamaClient) HasModel(name string) (bool, error) {
	resp, err := c.httpClient.Get(c.host + "/api/tags")
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	var result struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false, err
	}

	for _, m := range result.Models {
		if m.Name == name || m.Name == name+":latest" {
			return true, nil
		}
	}

	return false, nil
}
