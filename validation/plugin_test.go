package validation

import (
	"context"
	"testing"

	"google.golang.org/protobuf/proto"
)

type testPlugin struct {
	name        string
	entityTypes []string
	called      bool
}

func (p *testPlugin) Name() string         { return p.name }
func (p *testPlugin) EntityTypes() []string { return p.entityTypes }
func (p *testPlugin) Validate(_ context.Context, _ string, _ proto.Message) *ValidationResult {
	p.called = true
	r := NewValidResult()
	r.AddWarning("test", nil, "PLUGIN_TEST", "test warning", "check")
	return r
}

func TestPluginRegistry_Empty(t *testing.T) {
	pr := NewPluginRegistry()
	plugins := pr.ForEntityType("entities.v1.Person")
	if len(plugins) != 0 {
		t.Errorf("expected 0 plugins, got %d", len(plugins))
	}
}

func TestPluginRegistry_MatchByType(t *testing.T) {
	pr := NewPluginRegistry()
	pr.Register(&testPlugin{name: "email", entityTypes: []string{"entities.v1.Person"}})
	pr.Register(&testPlugin{name: "phone", entityTypes: []string{"entities.v1.Organisation"}})

	plugins := pr.ForEntityType("entities.v1.Person")
	if len(plugins) != 1 {
		t.Fatalf("expected 1 plugin, got %d", len(plugins))
	}
	if plugins[0].Name() != "email" {
		t.Errorf("expected email plugin, got %s", plugins[0].Name())
	}
}

func TestPluginRegistry_WildcardPlugin(t *testing.T) {
	pr := NewPluginRegistry()
	pr.Register(&testPlugin{name: "universal", entityTypes: nil})

	plugins := pr.ForEntityType("entities.v1.Person")
	if len(plugins) != 1 {
		t.Fatalf("expected 1 plugin, got %d", len(plugins))
	}
	if plugins[0].Name() != "universal" {
		t.Errorf("expected universal plugin, got %s", plugins[0].Name())
	}
}

func TestPluginRegistry_MultipleMatches(t *testing.T) {
	pr := NewPluginRegistry()
	pr.Register(&testPlugin{name: "email", entityTypes: []string{"entities.v1.Person", "entities.v1.Organisation"}})
	pr.Register(&testPlugin{name: "phone", entityTypes: []string{"entities.v1.Person"}})
	pr.Register(&testPlugin{name: "dedup", entityTypes: []string{"entities.v1.Address"}})

	plugins := pr.ForEntityType("entities.v1.Person")
	if len(plugins) != 2 {
		t.Fatalf("expected 2 plugins, got %d", len(plugins))
	}
}

func TestPluginRegistry_NoMatch(t *testing.T) {
	pr := NewPluginRegistry()
	pr.Register(&testPlugin{name: "email", entityTypes: []string{"entities.v1.Person"}})

	plugins := pr.ForEntityType("entities.v1.Invoice")
	if len(plugins) != 0 {
		t.Errorf("expected 0 plugins, got %d", len(plugins))
	}
}
