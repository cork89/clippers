package ollama

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

// Client handles communication with Ollama API
type Client struct {
	host       string
	httpClient *http.Client
}

// NewClient creates a new Ollama client
func NewClient(host string) *Client {
	return &Client{
		host: host,
		httpClient: &http.Client{
			Timeout: 5 * time.Minute, // Vision models can be slow
		},
	}
}

// GenerateRequest is the request body for /api/generate
type GenerateRequest struct {
	Model   string   `json:"model"`
	Prompt  string   `json:"prompt"`
	Images  []string `json:"images,omitempty"` // base64 encoded
	Stream  bool     `json:"stream"`
	Format  string   `json:"format,omitempty"` // "json" for JSON mode
	Options *Options `json:"options,omitempty"`
}

// Options for generation
type Options struct {
	Temperature float64 `json:"temperature,omitempty"`
	Seed        int     `json:"seed,omitempty"`
}

// GenerateResponse is the response from /api/generate
type GenerateResponse struct {
	Model    string `json:"model"`
	Response string `json:"response"`
	Done     bool   `json:"done"`
	Error    string `json:"error,omitempty"`
}

// Generate sends a generate request to Ollama
func (c *Client) Generate(req *GenerateRequest) (*GenerateResponse, error) {
	req.Stream = false // We don't handle streaming

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

	var result GenerateResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if result.Error != "" {
		return nil, fmt.Errorf("ollama error: %s", result.Error)
	}

	return &result, nil
}

// GenerateWithImage sends a vision request with an image
func (c *Client) GenerateWithImage(model, prompt, imagePath string, jsonMode bool) (string, error) {
	// Read and encode image
	imageData, err := os.ReadFile(imagePath)
	if err != nil {
		return "", fmt.Errorf("failed to read image: %w", err)
	}

	encoded := base64.StdEncoding.EncodeToString(imageData)

	req := &GenerateRequest{
		Model:  model,
		Prompt: prompt,
		Images: []string{encoded},
		Options: &Options{
			Temperature: 0,
		},
	}

	if jsonMode {
		req.Format = "json"
	}

	resp, err := c.Generate(req)
	if err != nil {
		return "", err
	}

	return resp.Response, nil
}

// GenerateText sends a text-only request
func (c *Client) GenerateText(model, prompt string, jsonMode bool) (string, error) {
	req := &GenerateRequest{
		Model:  model,
		Prompt: prompt,
		Options: &Options{
			Temperature: 0,
		},
	}

	if jsonMode {
		req.Format = "json"
	}

	resp, err := c.Generate(req)
	if err != nil {
		return "", err
	}

	return resp.Response, nil
}

// Ping checks if Ollama is reachable
func (c *Client) Ping() error {
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

// HasModel checks if a model is available
func (c *Client) HasModel(name string) (bool, error) {
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
