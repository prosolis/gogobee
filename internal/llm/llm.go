// Package llm wraps the local inference endpoint behind a backend-agnostic
// interface. Two concrete backends — Ollama (native /api/generate) and vLLM
// (OpenAI-compatible /v1/chat/completions) — implement Client; plugin code
// calls the interface only and never knows which one is active.
//
// Deliberately not routed through internal/safehttp: that client blocks
// RFC1918 and loopback destinations to defend against SSRF from feed-supplied
// URLs, and the inference endpoint is precisely such a destination. The URL
// here comes from our own config, never from user input.
package llm

import (
	"context"
	"os"
	"strings"
	"time"
)

// Request is the backend-neutral generation request. Zero-valued fields fall
// back to backend defaults.
type Request struct {
	// Prompt is a single raw instruction. The vLLM backend wraps it as one
	// user message so the model's chat template still applies; sending it to
	// /v1/completions instead would bypass the template and degrade an
	// instruction-tuned model badly.
	Prompt string
	// System is an optional system message. Empty means none, which keeps the
	// single-message shape the majority of callers use.
	System string
	// NumCtx is the per-request context window. Ollama honours it directly;
	// vLLM fixes the window server-side at launch (--max-model-len), so this
	// is ignored there rather than silently misapplied.
	NumCtx int
	// MaxTokens caps the completion length. 0 means the backend default.
	MaxTokens int
	// Temperature is passed through when non-zero.
	Temperature float64
	// Timeout overrides the client's default per-request budget.
	Timeout time.Duration
}

// Client is the single surface plugin code depends on.
type Client interface {
	// Generate returns the full completion in one shot. Reasoning blocks are
	// stripped before returning (see StripThink) — every caller in this repo
	// wants the visible answer, not the chain of thought.
	Generate(ctx context.Context, req Request) (string, error)
	// Model reports the configured model id, for logging and /botinfo.
	Model() string
	// Ping reports the model ids the backend is currently serving. Used by
	// /botinfo for a liveness line; the two backends expose this on different
	// paths (/api/tags vs /v1/models), which is exactly the sort of difference
	// this interface exists to hide.
	Ping(ctx context.Context) ([]string, error)
	// Backend reports "ollama" or "vllm", for logging and /botinfo.
	Backend() string
}

// Config selects and configures a backend.
type Config struct {
	Backend  string // "ollama" | "vllm"
	Endpoint string
	Model    string
	Timeout  time.Duration
}

// DefaultTimeout matches the budget the pre-refactor callOllama used.
const DefaultTimeout = 120 * time.Second

// ConfigFromEnv reads backend settings, preferring the new LLM_* names and
// falling back to the legacy OLLAMA_* pair so an existing deployment keeps
// working untouched after this refactor.
func ConfigFromEnv() Config {
	backend := strings.ToLower(strings.TrimSpace(os.Getenv("LLM_BACKEND")))
	if backend == "" {
		backend = "ollama"
	}

	endpoint := firstNonEmpty(os.Getenv("LLM_ENDPOINT"), os.Getenv("OLLAMA_HOST"))
	model := firstNonEmpty(os.Getenv("LLM_MODEL"), os.Getenv("OLLAMA_MODEL"))

	timeout := DefaultTimeout
	if d, err := time.ParseDuration(os.Getenv("LLM_TIMEOUT")); err == nil && d > 0 {
		timeout = d
	}

	return Config{Backend: backend, Endpoint: endpoint, Model: model, Timeout: timeout}
}

// Configured reports whether enough config is present to talk to a backend.
// Plugins check this to stay dormant rather than erroring on every invocation,
// which is what the old `if ollamaHost == "" || ollamaModel == ""` guards did.
func (c Config) Configured() bool {
	return c.Endpoint != "" && c.Model != ""
}

// New builds the client for cfg.Backend. An unrecognised backend falls back to
// Ollama, which is what every existing deployment runs.
func New(cfg Config) Client {
	if cfg.Timeout <= 0 {
		cfg.Timeout = DefaultTimeout
	}
	base := backend{
		endpoint: strings.TrimRight(cfg.Endpoint, "/"),
		model:    cfg.Model,
		timeout:  cfg.Timeout,
	}
	switch cfg.Backend {
	case "vllm":
		return &VLLMClient{base}
	default:
		return &OllamaClient{base}
	}
}

// backend holds the fields shared by both concrete clients.
type backend struct {
	endpoint string
	model    string
	timeout  time.Duration
}

func (b backend) Model() string { return b.model }

// timeoutFor lets a single call widen or narrow the client default. The two
// dispatch-voice callers rely on this: a dispatch is authored on a game
// chokepoint and must not stall it, while a run summary rides a background
// ticker and can afford a bigger model.
func (b backend) timeoutFor(req Request) time.Duration {
	if req.Timeout > 0 {
		return req.Timeout
	}
	return b.timeout
}

// StripThink removes a leading <think>...</think> reasoning block, which Qwen
// models emit even when thinking is disabled by some backends. Callers that
// parse JSON out of the completion depend on this running first.
func StripThink(s string) string {
	for {
		i := strings.Index(s, "<think>")
		if i < 0 {
			break
		}
		j := strings.Index(s, "</think>")
		if j < 0 || j < i {
			break
		}
		s = s[:i] + s[j+len("</think>"):]
	}
	return strings.TrimSpace(s)
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v = strings.TrimSpace(v); v != "" {
			return v
		}
	}
	return ""
}
