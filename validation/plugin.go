package validation

import (
	"context"

	"google.golang.org/protobuf/proto"
)

// Plugin defines a post-extraction validation concern.
type Plugin interface {
	// Name returns a unique identifier for the plugin.
	Name() string
	// EntityTypes returns the entity types this plugin applies to.
	// An empty slice means the plugin applies to all entity types.
	EntityTypes() []string
	// Validate runs the plugin's validation logic.
	Validate(ctx context.Context, entityType string, msg proto.Message) *ValidationResult
}

// PluginRegistry holds registered validation plugins.
type PluginRegistry struct {
	plugins []Plugin
}

// NewPluginRegistry creates an empty PluginRegistry.
func NewPluginRegistry() *PluginRegistry {
	return &PluginRegistry{}
}

// Register adds a plugin to the registry.
func (r *PluginRegistry) Register(p Plugin) {
	r.plugins = append(r.plugins, p)
}

// ForEntityType returns all plugins that apply to the given entity type.
func (r *PluginRegistry) ForEntityType(entityType string) []Plugin {
	var result []Plugin
	for _, p := range r.plugins {
		types := p.EntityTypes()
		if len(types) == 0 {
			result = append(result, p)
			continue
		}
		for _, t := range types {
			if t == entityType {
				result = append(result, p)
				break
			}
		}
	}
	return result
}
