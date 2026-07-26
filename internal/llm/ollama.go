package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// OllamaClient talks to Ollama's native /api/generate endpoint.
type OllamaClient struct {
	backend
}

func (c *OllamaClient) Backend() string { return "ollama" }

type ollamaOptions struct {
	NumCtx      int     `json:"num_ctx,omitempty"`
	NumPredict  int     `json:"num_predict,omitempty"`
	Temperature float64 `json:"temperature,omitempty"`
}

type ollamaRequest struct {
	Model   string        `json:"model"`
	Prompt  string        `json:"prompt"`
	System  string        `json:"system,omitempty"`
	Stream  bool          `json:"stream"`
	Think   bool          `json:"think"`
	Options ollamaOptions `json:"options,omitempty"`
}

// Generate posts a single non-streaming generation and returns the completion.
func (c *OllamaClient) Generate(ctx context.Context, req Request) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeoutFor(req))
	defer cancel()

	body, err := json.Marshal(ollamaRequest{
		Model:  c.model,
		Prompt: req.Prompt,
		System: req.System,
		Stream: false,
		Think:  false,
		Options: ollamaOptions{
			NumCtx:      req.NumCtx,
			NumPredict:  req.MaxTokens,
			Temperature: req.Temperature,
		},
	})
	if err != nil {
		return "", fmt.Errorf("ollama: marshal payload: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.endpoint+"/api/generate", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("ollama: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("ollama request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("ollama: read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ollama HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Response string `json:"response"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("ollama: parse response: %w", err)
	}
	return StripThink(result.Response), nil
}

// Ping lists locally installed models via Ollama's native /api/tags.
func (c *OllamaClient) Ping(ctx context.Context) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, pingTimeout)
	defer cancel()

	var out struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := getJSON(ctx, c.endpoint+"/api/tags", &out); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(out.Models))
	for _, m := range out.Models {
		names = append(names, m.Name)
	}
	return names, nil
}
