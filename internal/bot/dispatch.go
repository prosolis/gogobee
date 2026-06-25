package bot

import (
	"log/slog"
	"os"
	"strings"
	"sync"

	"gogobee/internal/plugin"
)

// Registry manages plugin registration and event dispatch.
type Registry struct {
	mu          sync.RWMutex
	plugins     []plugin.Plugin
	ignoredBots map[string]struct{}
}

// NewRegistry creates an empty plugin registry. Senders listed in the
// IGNORED_BOTS env var (comma-separated full Matrix user IDs, e.g.
// "@pete:parodia.dev") are dropped before any plugin sees them.
func NewRegistry() *Registry {
	ignored := make(map[string]struct{})
	if raw := os.Getenv("IGNORED_BOTS"); raw != "" {
		for _, u := range strings.Split(raw, ",") {
			if u = strings.TrimSpace(u); u != "" {
				ignored[u] = struct{}{}
			}
		}
	}
	return &Registry{ignoredBots: ignored}
}

// Register adds a plugin to the registry.
func (r *Registry) Register(p plugin.Plugin) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.plugins = append(r.plugins, p)
	slog.Info("registered plugin", "name", p.Name())
}

// Init initializes all registered plugins.
func (r *Registry) Init() error {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, p := range r.plugins {
		slog.Info("initializing plugin", "name", p.Name())
		if err := p.Init(); err != nil {
			return err
		}
		slog.Info("initialized plugin", "name", p.Name())
	}
	return nil
}

// DispatchMessage sends a message context to all plugins in order.
func (r *Registry) DispatchMessage(ctx plugin.MessageContext) {
	if _, ok := r.ignoredBots[string(ctx.Sender)]; ok {
		return
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, p := range r.plugins {
		r.safeOnMessage(p, ctx)
	}
}

func (r *Registry) safeOnMessage(p plugin.Plugin, ctx plugin.MessageContext) {
	defer func() {
		if rec := recover(); rec != nil {
			slog.Error("plugin message handler panic",
				"plugin", p.Name(),
				"panic", rec,
				"sender", ctx.Sender,
				"room", ctx.RoomID,
				"body", truncate(ctx.Body, 100),
			)
		}
	}()
	if err := p.OnMessage(ctx); err != nil {
		slog.Error("plugin message handler error",
			"plugin", p.Name(),
			"err", err,
		)
	}
}

// DispatchReaction sends a reaction context to all plugins in order.
func (r *Registry) DispatchReaction(ctx plugin.ReactionContext) {
	if _, ok := r.ignoredBots[string(ctx.Sender)]; ok {
		return
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, p := range r.plugins {
		r.safeOnReaction(p, ctx)
	}
}

func (r *Registry) safeOnReaction(p plugin.Plugin, ctx plugin.ReactionContext) {
	defer func() {
		if rec := recover(); rec != nil {
			slog.Error("plugin reaction handler panic",
				"plugin", p.Name(),
				"panic", rec,
				"sender", ctx.Sender,
				"room", ctx.RoomID,
				"emoji", ctx.Emoji,
			)
		}
	}()
	if err := p.OnReaction(ctx); err != nil {
		slog.Error("plugin reaction handler error",
			"plugin", p.Name(),
			"err", err,
		)
	}
}

// GetCommands returns all command definitions from all plugins.
func (r *Registry) GetCommands() []plugin.CommandDef {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var cmds []plugin.CommandDef
	for _, p := range r.plugins {
		cmds = append(cmds, p.Commands()...)
	}
	return cmds
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// GetPlugin returns a plugin by name.
func (r *Registry) GetPlugin(name string) plugin.Plugin {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, p := range r.plugins {
		if p.Name() == name {
			return p
		}
	}
	return nil
}
