package validation

import (
	"context"
	"testing"

	"google.golang.org/protobuf/proto"
)

func TestValidatorRegistry_NoValidator(t *testing.T) {
	vr := NewValidatorRegistry()
	result := vr.Validate(context.Background(), "entities.v1.Unknown", nil)
	if !result.Valid {
		t.Error("expected valid result for unregistered type")
	}
	if len(result.Findings) != 0 {
		t.Errorf("expected no findings, got %d", len(result.Findings))
	}
}

func TestValidatorRegistry_RegisterAndValidate(t *testing.T) {
	vr := NewValidatorRegistry()
	called := false
	vr.Register("entities.v1.Test", func(_ context.Context, _ proto.Message) *ValidationResult {
		called = true
		r := NewValidResult()
		r.AddError("name", "", "REQUIRED", "name required", "add name")
		return r
	})

	result := vr.Validate(context.Background(), "entities.v1.Test", nil)
	if !called {
		t.Error("expected validator to be called")
	}
	if result.Valid {
		t.Error("expected invalid result")
	}
	if len(result.Findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(result.Findings))
	}
	if result.Findings[0].Code != "REQUIRED" {
		t.Errorf("expected code REQUIRED, got %s", result.Findings[0].Code)
	}
}

func TestValidatorRegistry_OverwriteValidator(t *testing.T) {
	vr := NewValidatorRegistry()
	vr.Register("entities.v1.Test", func(_ context.Context, _ proto.Message) *ValidationResult {
		return NewValidResult()
	})
	vr.Register("entities.v1.Test", func(_ context.Context, _ proto.Message) *ValidationResult {
		r := NewValidResult()
		r.AddWarning("x", nil, "W", "warning", "check")
		return r
	})

	result := vr.Validate(context.Background(), "entities.v1.Test", nil)
	if len(result.Findings) != 1 {
		t.Fatalf("expected 1 finding from overwritten validator, got %d", len(result.Findings))
	}
}
