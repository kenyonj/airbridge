package plugins

import (
	"fmt"
	"sync"
)

// Registry holds registered plugin factories and active plugin instances.
type Registry struct {
	mu        sync.RWMutex
	factories map[string]PluginFactory
	instances map[string]VolumePlugin // keyed by plugin config ID
}

// NewRegistry creates a new plugin registry.
func NewRegistry() *Registry {
	return &Registry{
		factories: make(map[string]PluginFactory),
		instances: make(map[string]VolumePlugin),
	}
}

// RegisterFactory registers a plugin factory for a given type.
func (r *Registry) RegisterFactory(pluginType string, factory PluginFactory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.factories[pluginType] = factory
}

// GetFactory returns the factory for a plugin type.
func (r *Registry) GetFactory(pluginType string) (PluginFactory, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	f, ok := r.factories[pluginType]
	return f, ok
}

// ListTypes returns all registered plugin types.
func (r *Registry) ListTypes() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	types := make([]string, 0, len(r.factories))
	for t := range r.factories {
		types = append(types, t)
	}
	return types
}

// CreateInstance creates and registers a plugin instance from config.
func (r *Registry) CreateInstance(cfg PluginConfig) (VolumePlugin, error) {
	factory, ok := r.GetFactory(cfg.Type)
	if !ok {
		return nil, fmt.Errorf("unknown plugin type: %s", cfg.Type)
	}

	plugin, err := factory(cfg.Config)
	if err != nil {
		return nil, fmt.Errorf("failed to create plugin: %w", err)
	}

	r.mu.Lock()
	r.instances[cfg.ID] = plugin
	r.mu.Unlock()

	return plugin, nil
}

// GetInstance returns an active plugin instance by config ID.
func (r *Registry) GetInstance(configID string) (VolumePlugin, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.instances[configID]
	return p, ok
}

// RemoveInstance removes a plugin instance.
func (r *Registry) RemoveInstance(configID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.instances, configID)
}

// ListInstances returns all active plugin instance IDs.
func (r *Registry) ListInstances() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ids := make([]string, 0, len(r.instances))
	for id := range r.instances {
		ids = append(ids, id)
	}
	return ids
}

// GetInstances returns a copy of all active plugin instances.
func (r *Registry) GetInstances() map[string]VolumePlugin {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make(map[string]VolumePlugin, len(r.instances))
	for id, p := range r.instances {
		result[id] = p
	}
	return result
}
