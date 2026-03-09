package registry

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/laenen-partners/llmextract/schema"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// Registry holds all known entity types, their JSON schemas, and factory functions.
type Registry struct {
	descriptors map[protoreflect.FullName]protoreflect.MessageDescriptor
	schemas     map[protoreflect.FullName]map[string]any
	factories   map[protoreflect.FullName]func() proto.Message
	order       []protoreflect.FullName // preserves registration order
}

// New creates an empty Registry.
func New() *Registry {
	return &Registry{
		descriptors: make(map[protoreflect.FullName]protoreflect.MessageDescriptor),
		schemas:     make(map[protoreflect.FullName]map[string]any),
		factories:   make(map[protoreflect.FullName]func() proto.Message),
	}
}

// Register adds an entity type to the registry. The provided message is used as
// a prototype for creating new instances and deriving the schema.
func (r *Registry) Register(msg proto.Message) {
	md := msg.ProtoReflect().Descriptor()
	fullName := md.FullName()

	r.descriptors[fullName] = md
	r.schemas[fullName] = schema.ProtoToJSONSchema(md)
	r.factories[fullName] = func() proto.Message {
		return proto.Clone(msg)
	}
	r.order = append(r.order, fullName)
}

// Schema returns the JSON Schema for a registered entity type.
func (r *Registry) Schema(name protoreflect.FullName) (map[string]any, bool) {
	s, ok := r.schemas[name]
	return s, ok
}

// NewInstance creates a new zero-value instance of the registered entity type.
func (r *Registry) NewInstance(name protoreflect.FullName) (proto.Message, error) {
	factory, ok := r.factories[name]
	if !ok {
		return nil, fmt.Errorf("unknown entity type: %s", name)
	}
	return factory(), nil
}

// AllTypes returns all registered entity type names in registration order.
func (r *Registry) AllTypes() []protoreflect.FullName {
	return r.order
}

// SchemasSummary produces a text summary of all registered schemas for use in LLM prompts.
func (r *Registry) SchemasSummary() string {
	var b strings.Builder
	for _, name := range r.order {
		s := r.schemas[name]
		jsonBytes, _ := json.MarshalIndent(s, "", "  ")
		fmt.Fprintf(&b, "### %s\n```json\n%s\n```\n\n", name, string(jsonBytes))
	}
	return b.String()
}

// HasType checks if an entity type name (as a string) is registered.
func (r *Registry) HasType(name string) bool {
	_, ok := r.schemas[protoreflect.FullName(name)]
	return ok
}

// Descriptor returns the proto descriptor for a registered entity type.
func (r *Registry) Descriptor(name protoreflect.FullName) (protoreflect.MessageDescriptor, bool) {
	md, ok := r.descriptors[name]
	return md, ok
}
