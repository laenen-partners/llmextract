package validation

import (
	"testing"
)

func TestNewValidResult(t *testing.T) {
	r := NewValidResult()
	if !r.Valid {
		t.Error("expected Valid=true")
	}
	if len(r.Findings) != 0 {
		t.Error("expected no findings")
	}
}

func TestAddError(t *testing.T) {
	r := NewValidResult()
	r.AddError("email", "bad@", "INVALID_EMAIL", "bad email", "fix it")

	if r.Valid {
		t.Error("expected Valid=false after AddError")
	}
	if len(r.Findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(r.Findings))
	}
	f := r.Findings[0]
	if f.Severity != SeverityError {
		t.Errorf("expected severity error, got %s", f.Severity)
	}
	if f.Field != "email" {
		t.Errorf("expected field email, got %s", f.Field)
	}
	if f.Code != "INVALID_EMAIL" {
		t.Errorf("expected code INVALID_EMAIL, got %s", f.Code)
	}
}

func TestAddWarning(t *testing.T) {
	r := NewValidResult()
	r.AddWarning("age", 35, "AGE_MISMATCH", "age off", "check it")

	if !r.Valid {
		t.Error("warnings should not invalidate the result")
	}
	if len(r.Findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(r.Findings))
	}
	if r.Findings[0].Severity != SeverityWarning {
		t.Errorf("expected severity warning, got %s", r.Findings[0].Severity)
	}
}

func TestMerge(t *testing.T) {
	parent := NewValidResult()
	child := NewValidResult()
	child.AddError("city", "", "REQUIRED", "city required", "add city")

	parent.Merge("address", child)

	if parent.Valid {
		t.Error("parent should be invalid after merging invalid child")
	}
	if len(parent.Findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(parent.Findings))
	}
	if parent.Findings[0].Field != "address.city" {
		t.Errorf("expected field 'address.city', got '%s'", parent.Findings[0].Field)
	}
}

func TestMergeEmptyPrefix(t *testing.T) {
	parent := NewValidResult()
	child := NewValidResult()
	child.AddError("name", "", "REQUIRED", "required", "fix")

	parent.Merge("", child)

	if parent.Findings[0].Field != "name" {
		t.Errorf("expected field 'name', got '%s'", parent.Findings[0].Field)
	}
}

func TestWithSuggestion(t *testing.T) {
	r := NewValidResult()
	r.AddError("age", 35, "AGE_MISMATCH", "age off", "fix", WithSuggestion(32))

	if r.Findings[0].Suggestion != 32 {
		t.Errorf("expected suggestion 32, got %v", r.Findings[0].Suggestion)
	}
}

func TestWithConstraints(t *testing.T) {
	r := NewValidResult()
	r.AddError("email", "x", "BAD", "bad", "fix", WithConstraints("RFC5322", "min_len=5"))

	c := r.Findings[0].Constraints
	if len(c) != 2 || c[0] != "RFC5322" {
		t.Errorf("expected constraints [RFC5322 min_len=5], got %v", c)
	}
}

func TestHasErrors(t *testing.T) {
	r := NewValidResult()
	if r.HasErrors() {
		t.Error("should not have errors when empty")
	}
	r.AddWarning("x", nil, "W", "w", "w")
	if r.HasErrors() {
		t.Error("warnings are not errors")
	}
	r.AddError("y", nil, "E", "e", "e")
	if !r.HasErrors() {
		t.Error("should have errors")
	}
}

func TestErrorCount(t *testing.T) {
	r := NewValidResult()
	r.AddError("a", nil, "E1", "e1", "fix")
	r.AddWarning("b", nil, "W1", "w1", "check")
	r.AddError("c", nil, "E2", "e2", "fix")

	if r.ErrorCount() != 2 {
		t.Errorf("expected 2 errors, got %d", r.ErrorCount())
	}
}

func TestJSON(t *testing.T) {
	r := NewValidResult()
	r.AddError("email", "bad@", "INVALID_EMAIL", "bad email", "fix it")

	data, err := r.JSON()
	if err != nil {
		t.Fatalf("JSON() error: %v", err)
	}
	if len(data) == 0 {
		t.Error("expected non-empty JSON")
	}
}

func TestWithCalculation(t *testing.T) {
	chain := &CalculationChain{
		Name: "invoice_total_chain",
		Steps: []ArithmeticStep{
			{Label: "line_items_sum", Formula: "sum(line_items[i].amount)", Expected: 100.0, Actual: 100.0, Field: "subtotal"},
			{Label: "total_calc", Formula: "subtotal - discount + tax + shipping", Expected: 121.0, Actual: 100.0, Field: "total_amount"},
		},
		Final: "total = subtotal - discount + tax + shipping",
	}
	r := NewValidResult()
	r.AddError("total_amount", 100.0, "TOTAL_AMOUNT_MISMATCH", "total mismatch", "fix total", WithCalculation(chain))

	f := r.Findings[0]
	if f.Calculation == nil {
		t.Fatal("expected Calculation to be set")
	}
	if f.Calculation.Name != "invoice_total_chain" {
		t.Errorf("expected chain name 'invoice_total_chain', got %q", f.Calculation.Name)
	}
	if len(f.Calculation.Steps) != 2 {
		t.Errorf("expected 2 steps, got %d", len(f.Calculation.Steps))
	}
}

func TestWithRelatedFields(t *testing.T) {
	r := NewValidResult()
	r.AddError("total_amount", 100.0, "TOTAL_AMOUNT_MISMATCH", "mismatch", "fix",
		WithRelatedFields("subtotal", "tax_amount", "discount_amount"))

	f := r.Findings[0]
	if len(f.RelatedFields) != 3 {
		t.Fatalf("expected 3 related fields, got %d", len(f.RelatedFields))
	}
	if f.RelatedFields[0] != "subtotal" {
		t.Errorf("expected first related field 'subtotal', got %q", f.RelatedFields[0])
	}
}
