package runner

import (
	"context"
	"encoding/json"
	"fmt"
)

// StepRunner executes named steps. Implementations provide durability, tracing, or both.
type StepRunner interface {
	// Run executes a named step. The name is used for idempotency keys,
	// trace spans, and activity registration depending on the backend.
	Run(ctx context.Context, name string, fn func(ctx context.Context) (any, error)) (any, error)
}

// RunStep is a generic helper that bridges typed step functions to the any-typed StepRunner interface.
func RunStep[O any](ctx context.Context, r StepRunner, name string, fn func(ctx context.Context) (O, error)) (O, error) {
	raw, err := r.Run(ctx, name, func(ctx context.Context) (any, error) {
		return fn(ctx)
	})
	if err != nil {
		var zero O
		return zero, err
	}
	if typed, ok := raw.(O); ok {
		return typed, nil
	}
	// Durable runners may return deserialized JSON (e.g. map[string]any) — try JSON round-trip.
	var result O
	b, _ := json.Marshal(raw)
	if err := json.Unmarshal(b, &result); err != nil {
		var zero O
		return zero, fmt.Errorf("step %q: cannot convert result to %T: %w", name, result, err)
	}
	return result, nil
}
