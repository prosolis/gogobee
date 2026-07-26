package plugin

import (
	"context"
	"sync"

	"gogobee/internal/llm"
)

// The inference backend is process-wide: one endpoint, one model, selected by
// env at startup. Plugins share a single client rather than each rebuilding one
// per invocation, and read config through llmConfigured/llmGenerate so that
// swapping Ollama for vLLM is a config change rather than a code change.
var (
	llmOnce   sync.Once
	llmShared llm.Client
	llmCfg    llm.Config
)

func llmInit() {
	llmOnce.Do(func() {
		llmCfg = llm.ConfigFromEnv()
		llmShared = llm.New(llmCfg)
	})
}

// llmConfigured reports whether an endpoint and model are set. Plugins call
// this to stay dormant instead of erroring on every invocation — the same role
// the old `if ollamaHost == "" || ollamaModel == ""` guards played.
func llmConfigured() bool {
	llmInit()
	return llmCfg.Configured()
}

// llmClient returns the shared backend client.
func llmClient() llm.Client {
	llmInit()
	return llmShared
}

// llmGenerate is the one-line path for the common case: a raw prompt in,
// visible completion out, reasoning blocks already stripped.
func llmGenerate(ctx context.Context, req llm.Request) (string, error) {
	return llmClient().Generate(ctx, req)
}
