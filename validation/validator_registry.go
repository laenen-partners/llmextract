package validation

import (
	"context"

	"google.golang.org/protobuf/proto"
)

// ValidatorFunc performs semantic validation on a proto message.
type ValidatorFunc func(ctx context.Context, msg proto.Message) *ValidationResult

// ValidatorRegistry maps entity type names to validator functions.
type ValidatorRegistry struct {
	validators map[string]ValidatorFunc
}

// NewValidatorRegistry creates an empty ValidatorRegistry.
func NewValidatorRegistry() *ValidatorRegistry {
	return &ValidatorRegistry{
		validators: make(map[string]ValidatorFunc),
	}
}

// Register adds a validator function for the given entity type.
func (r *ValidatorRegistry) Register(entityType string, fn ValidatorFunc) {
	r.validators[entityType] = fn
}

// Validate runs the registered validator for the given entity type.
// If no validator is registered, returns a valid result (no-op).
func (r *ValidatorRegistry) Validate(ctx context.Context, entityType string, msg proto.Message) *ValidationResult {
	fn, ok := r.validators[entityType]
	if !ok {
		return NewValidResult()
	}
	return fn(ctx, msg)
}
