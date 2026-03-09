package validation

import (
	"encoding/json"
)

// Severity indicates how serious a validation finding is.
type Severity string

const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
	SeverityInfo    Severity = "info"
)

// ValidationResult holds the outcome of validating an entity.
type ValidationResult struct {
	Valid    bool                `json:"valid"`
	Findings []ValidationFinding `json:"findings"`
}

// ArithmeticStep records one step in an arithmetic verification chain.
type ArithmeticStep struct {
	Label    string  `json:"label"`
	Formula  string  `json:"formula"`
	Expected float64 `json:"expected"`
	Actual   float64 `json:"actual"`
	Field    string  `json:"field"`
}

// CalculationChain groups arithmetic steps for a named verification chain.
type CalculationChain struct {
	Name  string           `json:"name"`
	Steps []ArithmeticStep `json:"steps"`
	Final string           `json:"final"`
}

// ValidationFinding describes a single validation issue.
type ValidationFinding struct {
	Field         string            `json:"field"`
	Value         any               `json:"value"`
	Severity      Severity          `json:"severity"`
	Code          string            `json:"code"`
	Message       string            `json:"message"`
	Action        string            `json:"action"`
	Suggestion    any               `json:"suggestion,omitempty"`
	Constraints   []string          `json:"constraints,omitempty"`
	Calculation   *CalculationChain `json:"calculation,omitempty"`
	RelatedFields []string          `json:"related_fields,omitempty"`
}

// FindingOption configures optional fields on a ValidationFinding.
type FindingOption func(*ValidationFinding)

// WithSuggestion sets a suggested corrected value.
func WithSuggestion(s any) FindingOption {
	return func(f *ValidationFinding) { f.Suggestion = s }
}

// WithConstraints sets the constraint descriptions that were violated.
func WithConstraints(c ...string) FindingOption {
	return func(f *ValidationFinding) { f.Constraints = c }
}

// WithCalculation attaches an arithmetic verification chain to a finding.
func WithCalculation(c *CalculationChain) FindingOption {
	return func(f *ValidationFinding) { f.Calculation = c }
}

// WithRelatedFields marks other fields that share the same root cause.
func WithRelatedFields(fields ...string) FindingOption {
	return func(f *ValidationFinding) { f.RelatedFields = fields }
}

func (r *ValidationResult) add(sev Severity, field string, value any, code, message, action string, opts ...FindingOption) {
	f := ValidationFinding{
		Field:    field,
		Value:    value,
		Severity: sev,
		Code:     code,
		Message:  message,
		Action:   action,
	}
	for _, opt := range opts {
		opt(&f)
	}
	r.Findings = append(r.Findings, f)
}

// AddError adds an error-level finding and marks the result as invalid.
func (r *ValidationResult) AddError(field string, value any, code, message, action string, opts ...FindingOption) {
	r.Valid = false
	r.add(SeverityError, field, value, code, message, action, opts...)
}

// AddWarning adds a warning-level finding.
func (r *ValidationResult) AddWarning(field string, value any, code, message, action string, opts ...FindingOption) {
	r.add(SeverityWarning, field, value, code, message, action, opts...)
}

// AddInfo adds an informational finding.
func (r *ValidationResult) AddInfo(field string, value any, code, message, action string, opts ...FindingOption) {
	r.add(SeverityInfo, field, value, code, message, action, opts...)
}

// Merge incorporates findings from a nested entity's validation result,
// prefixing all field paths with the given prefix.
func (r *ValidationResult) Merge(prefix string, nested *ValidationResult) {
	if nested == nil {
		return
	}
	for _, f := range nested.Findings {
		switch {
		case prefix == "":
			// keep f.Field as-is
		case f.Field != "":
			f.Field = prefix + "." + f.Field
		default:
			f.Field = prefix
		}
		r.Findings = append(r.Findings, f)
	}
	if !nested.Valid {
		r.Valid = false
	}
}

// HasErrors returns true if any finding has error severity.
func (r *ValidationResult) HasErrors() bool {
	for _, f := range r.Findings {
		if f.Severity == SeverityError {
			return true
		}
	}
	return false
}

// ErrorCount returns the number of error-level findings.
func (r *ValidationResult) ErrorCount() int {
	count := 0
	for _, f := range r.Findings {
		if f.Severity == SeverityError {
			count++
		}
	}
	return count
}

// JSON serializes the result to JSON bytes.
func (r *ValidationResult) JSON() ([]byte, error) {
	return json.MarshalIndent(r, "", "  ")
}

// NewValidResult creates a valid result with no findings.
func NewValidResult() *ValidationResult {
	return &ValidationResult{Valid: true}
}
