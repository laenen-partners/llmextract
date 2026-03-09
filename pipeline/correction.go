package pipeline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"

	"buf.build/go/protovalidate"
	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/reflect/protoreflect"

	llmextract "github.com/laenen-partners/llmextract"
	"github.com/laenen-partners/llmextract/validation"
)

type correctionOutput struct {
	Entities    []llmextract.ExtractedEntity `json:"entities"`
	Corrections []llmextract.CorrectionEntry `json:"corrections"`
	TotalRounds int                          `json:"total_rounds"`
}

func (p *Pipeline) validateAndCorrect(ctx context.Context, document string, entities []llmextract.ExtractedEntity) (*correctionOutput, error) {
	result := &correctionOutput{
		Entities: make([]llmextract.ExtractedEntity, len(entities)),
	}
	copy(result.Entities, entities)

	validator, err := protovalidate.New()
	if err != nil {
		return nil, fmt.Errorf("creating protovalidate validator: %w", err)
	}

	for i := range result.Entities {
		e := &result.Entities[i]
		for round := 0; round < p.config.MaxCorrectionRounds; round++ {
			// Unmarshal into proto message
			entityType := protoreflect.FullName(e.EntityType)
			msg, err := p.registry.NewInstance(entityType)
			if err != nil {
				return nil, fmt.Errorf("creating instance for %s: %w", e.EntityType, err)
			}

			// Normalize date-only strings to RFC 3339 for Timestamp fields
			// before unmarshalling (smaller models often omit the time portion).
			md, _ := p.registry.Descriptor(entityType)
			normalized, err := normalizeTimestamps(e.Data, md)
			if err == nil {
				e.Data = normalized
			}

			if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(e.Data, msg); err != nil {
				return nil, fmt.Errorf("unmarshalling %s data: %w", e.EntityType, err)
			}

			// Layer 1: protovalidate annotations
			vr := validation.NewValidResult()
			if err := validator.Validate(msg); err != nil {
				mergeProtovalidateErrors(err, vr)
			}

			// Layer 2: semantic validation (via validator registry)
			if p.validators != nil {
				semanticResult := p.validators.Validate(ctx, e.EntityType, msg)
				vr.Merge("", semanticResult)
			}

			// Layer 3: plugin validation
			if p.plugins != nil {
				for _, plugin := range p.plugins.ForEntityType(e.EntityType) {
					pluginResult := plugin.Validate(ctx, e.EntityType, msg)
					if pluginResult != nil {
						vr.Merge("", pluginResult)
					}
				}
			}

			// If valid, we're done with this entity
			if vr.Valid {
				break
			}

			result.TotalRounds++
			e.Lineage.CorrectionRound = round + 1

			// Re-prompt the LLM with the validation findings
			corrected, corrections, err := p.correctEntity(ctx, document, e, vr)
			if err != nil {
				// If correction fails, keep the entity as-is and move on
				break
			}

			result.Corrections = append(result.Corrections, corrections...)
			e.Data = corrected
		}
	}

	return result, nil
}

func (p *Pipeline) correctEntity(ctx context.Context, document string, entity *llmextract.ExtractedEntity, vr *validation.ValidationResult) (json.RawMessage, []llmextract.CorrectionEntry, error) {
	findingsJSON, _ := vr.JSON()

	entityType := protoreflect.FullName(entity.EntityType)
	entitySchema, _ := p.registry.Schema(entityType)
	schemaJSON, _ := json.MarshalIndent(entitySchema, "", "  ")

	arithmeticCtx := buildArithmeticContext(vr)

	prompt := fmt.Sprintf(`You previously extracted the following %s entity from the document, but validation found issues.

Current entity data:
%s

Validation findings:
%s
%s
Original document:
---
%s
---

Please correct the entity based on the findings above. For each finding, follow the action instruction.
Output the corrected entity as JSON matching this schema:
%s`, entity.EntityType, string(entity.Data), string(findingsJSON), arithmeticCtx, document, string(schemaJSON))

	outputSchema := entitySchema

	generateOpts := []ai.GenerateOption{
		ai.WithModelName(p.modelName),
		ai.WithSystem("You are correcting entity extraction errors. Output only the corrected entity JSON."),
		ai.WithOutputSchema(outputSchema),
		ai.WithPrompt(prompt),
		p.generationConfig(),
	}
	if len(p.tools) > 0 {
		generateOpts = append(generateOpts, ai.WithTools(p.tools...), ai.WithMaxTurns(2))
	}
	resp, err := genkit.Generate(ctx, p.g, generateOpts...)
	if err != nil {
		return nil, nil, fmt.Errorf("LLM correction call: %w", err)
	}
	p.collectUsage(resp, "correct_"+entity.EntityType)
	p.collectToolCalls(resp, "correct_"+entity.EntityType)

	correctedData := json.RawMessage(resp.Text())

	// Build correction entries by comparing old and new
	var corrections []llmextract.CorrectionEntry
	for _, f := range vr.Findings {
		if f.Severity == validation.SeverityError {
			corrections = append(corrections, llmextract.CorrectionEntry{
				Round:    entity.Lineage.CorrectionRound,
				EntityID: entity.EntityID,
				Field:    f.Field,
				Reason:   f.Message,
			})
		}
	}

	return correctedData, corrections, nil
}

func buildArithmeticContext(vr *validation.ValidationResult) string {
	seen := map[string]bool{}
	var chains []*validation.CalculationChain
	for _, f := range vr.Findings {
		if f.Calculation != nil && !seen[f.Calculation.Name] {
			seen[f.Calculation.Name] = true
			chains = append(chains, f.Calculation)
		}
	}
	if len(chains) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("\nArithmetic verification (fix these calculations in order):\n")
	for _, ch := range chains {
		fmt.Fprintf(&b, "Chain: %s\n", ch.Name)
		fmt.Fprintf(&b, "Expected formula: %s\n", ch.Final)
		for i, step := range ch.Steps {
			status := "OK"
			if math.Abs(step.Expected-step.Actual) > 0.02 {
				status = fmt.Sprintf("MISMATCH (expected %.2f, got %.2f)", step.Expected, step.Actual)
			}
			fmt.Fprintf(&b, "  Step %d: %s = %s => %s\n", i+1, step.Label, step.Formula, status)
		}
	}
	b.WriteString("\nIMPORTANT: Fix calculations from the bottom up. Start with line item amounts, then subtotal, then tax, then total.\n\n")
	return b.String()
}

func mergeProtovalidateErrors(err error, vr *validation.ValidationResult) {
	var valErr *protovalidate.ValidationError
	if !errors.As(err, &valErr) {
		vr.AddError("", nil, "PROTO_VALIDATION_ERROR", err.Error(),
			"Fix the entity to conform to the schema constraints.")
		return
	}

	for _, v := range valErr.Violations {
		field := ""
		if fp := v.Proto.GetField(); fp != nil {
			for _, elem := range fp.GetElements() {
				if field != "" {
					field += "."
				}
				field += elem.GetFieldName()
			}
		}
		vr.AddError(field, nil,
			"PROTO_CONSTRAINT_"+v.Proto.GetRuleId(),
			v.Proto.GetMessage(),
			fmt.Sprintf("Fix the '%s' field to satisfy the constraint: %s", field, v.Proto.GetMessage()),
		)
	}
}
