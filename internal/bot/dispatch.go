package bot

import (
	"log/slog"
	"sync"

	"gogobee/internal/plugin"
)

// Registry manages plugin registration and event dispatch.
type Registry struct {
	mu      sync.RWMutex
	plugins []plugin.Plugin
}

// NewRegistry creates an empty plugin registry.
func NewRegistry() *Registry {
	return &Registry{}
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
		if err := p.Init(); err != nil {
			return err
		}
	}
	return nil
}

// DispatchMessage sends a message context to all plugins in order.
func (r *Registry) DispatchMessage(ctx plugin.MessageContext) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, p := range r.plugins {
		if err := p.OnMessage(ctx); err != nil {
			slog.Error("plugin message handler error",
				"plugin", p.Name(),
				"err", err,
			)
		}
	}
}

// DispatchReaction sends a reaction context to all plugins in order.
func (r *Registry) DispatchReaction(ctx plugin.ReactionContext) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, p := range r.plugins {
		if err := p.OnReaction(ctx); err != nil {
			slog.Error("plugin reaction handler error",
				"plugin", p.Name(),
				"err", err,
			)
		}
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
