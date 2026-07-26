package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// VLLMClient talks to an OpenAI-compatible /v1/chat/completions endpoint.
type VLLMClient struct {
	backend
}

func (c *VLLMClient) Backend() string { return "vllm" }

type vllmMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type vllmRequest struct {
	Model       string        `json:"model"`
	Messages    []vllmMessage `json:"messages"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
	Temperature float64       `json:"temperature,omitempty"`
	Stream      bool          `json:"stream"`
	// ChatTemplateKwargs is a vLLM extension to the OpenAI schema. It is how
	// Qwen3-family reasoning is switched off; the Ollama backend spells the
	// same intent as its native "think": false.
	ChatTemplateKwargs map[string]any `json:"chat_template_kwargs,omitempty"`
}

// Generate posts a single non-streaming completion and returns the message
// content. The raw prompt is sent as one user message so the server-side chat
// template still wraps it.
func (c *VLLMClient) Generate(ctx context.Context, req Request) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeoutFor(req))
	defer cancel()

	msgs := make([]vllmMessage, 0, 2)
	if req.System != "" {
		msgs = append(msgs, vllmMessage{Role: "system", Content: req.System})
	}
	msgs = append(msgs, vllmMessage{Role: "user", Content: req.Prompt})

	body, err := json.Marshal(vllmRequest{
		Model:              c.model,
		Messages:           msgs,
		MaxTokens:          req.MaxTokens,
		Temperature:        req.Temperature,
		Stream:             false,
		ChatTemplateKwargs: map[string]any{"enable_thinking": false},
	})
	if err != nil {
		return "", fmt.Errorf("vllm: marshal payload: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.endpoint+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("vllm: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("vllm request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("vllm: read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("vllm HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("vllm: parse response: %w", err)
	}
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("vllm: empty choices in response")
	}
	return StripThink(result.Choices[0].Message.Content), nil
}

// Ping lists served models via the OpenAI-compatible /v1/models.
func (c *VLLMClient) Ping(ctx context.Context) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, pingTimeout)
	defer cancel()

	var out struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := getJSON(ctx, c.endpoint+"/v1/models", &out); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(out.Data))
	for _, m := range out.Data {
		names = append(names, m.ID)
	}
	return names, nil
}
