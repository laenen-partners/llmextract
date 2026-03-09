// Package dbos provides a runner.StepRunner backed by DBOS durable steps.
//
// Each pipeline step is executed via dbos.RunAsStep, giving you automatic
// retries, exactly-once guarantees, and step-level checkpointing in Postgres.
package dbos

import (
	"context"

	"github.com/dbos-inc/dbos-transact-golang/dbos"
)

// Runner implements runner.StepRunner using DBOS durable steps.
type Runner struct {
	ctx dbos.DBOSContext
}

// New creates a Runner that dispatches pipeline steps via dbos.RunAsStep.
func New(ctx dbos.DBOSContext) *Runner {
	return &Runner{ctx: ctx}
}

// Run implements runner.StepRunner.
func (r *Runner) Run(ctx context.Context, name string, fn func(ctx context.Context) (any, error)) (any, error) {
	return dbos.RunAsStep(r.ctx, fn, dbos.WithStepName(name))
}
