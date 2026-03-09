package direct

import "context"

// Runner executes steps by calling the function directly. No durability or tracing.
type Runner struct{}

// New creates a new DirectRunner.
func New() *Runner {
	return &Runner{}
}

// Run calls fn directly.
func (r *Runner) Run(ctx context.Context, _ string, fn func(ctx context.Context) (any, error)) (any, error) {
	return fn(ctx)
}
