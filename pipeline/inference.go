package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"

	llmextract "github.com/laenen-partners/llmextract"
)

type inferenceResult struct {
	ImpliedEntities  []llmextract.ImpliedEntity  `json:"implied_entities"`
	ImpliedRelations []llmextract.EntityRelation `json:"implied_relations"`
}

// buildInferenceSystemPrompt constructs the inference system prompt using only
// the provided allowed relation types. If allowedTypes is empty, all types are used.
func buildInferenceSystemPrompt(allowedTypes []string) string {
	types := allowedTypes
	if len(types) == 0 {
		types = make([]string, 0, len(relationTypeDescriptions))
		for t := range relationTypeDescriptions {
			types = append(types, t)
		}
		sort.Strings(types)
	}

	var b strings.Builder
	b.WriteString(`You are a contextual inference system. Given a document and its extracted entities with roles, you must discover IMPLICIT information that was NOT directly extracted.

Your tasks:

1. COREFERENCE RESOLUTION
   Resolve pronouns and possessives ("my", "our", "his", "her", "their", "we") to the extracted entities.
   Use document roles (sender, recipient) to disambiguate:
   - "my" in a letter typically refers to the sender
   - "your" typically refers to the recipient
   - "our" refers to the sender's group/organization
   - "his"/"her" refers to the most recently mentioned person of that gender

2. IMPLIED ENTITIES
   Identify entities the text implies but that were NOT already extracted:
   - Events: "party", "meeting", "dinner", "trip", "conference"
   - Groups: "family", "team", "department", "committee"
   - People referenced by role: "his assistant", "our manager", "the accountant"
   Do NOT duplicate entities that were already extracted.

3. IMPLIED RELATIONS
   Identify relationships implied by language patterns:
   - Possessives implying family: "my daughter" -> parent_of
   - Relational nouns: "colleague" -> affiliated_with, "assistant" -> reports_to
   - Prepositional purpose: "party for [person]" -> honoree_of
   - Group membership: "our team" -> part_of
   - Event participation: "[person] will attend" -> participant_in

Relation types available:
`)
	b.WriteString("  ")
	b.WriteString(strings.Join(types, ", "))
	return b.String()
}

var inferenceOutputSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"implied_entities": map[string]any{
			"type": "array",
			"items": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"entity_type":   map[string]any{"type": "string", "description": "Free-form type: event, group, person, etc."},
					"description":   map[string]any{"type": "string", "description": "Human-readable description"},
					"properties":    map[string]any{"type": "object", "description": "Key-value pairs from context"},
					"inferred_from": map[string]any{"type": "string", "description": "The source text that implies this entity"},
					"confidence":    map[string]any{"type": "number"},
				},
				"required": []string{"entity_type", "description", "inferred_from", "confidence"},
			},
		},
		"implied_relations": map[string]any{
			"type": "array",
			"items": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"source_id":     map[string]any{"type": "string", "description": "entity_id of the source (extracted or implied_N)"},
					"target_id":     map[string]any{"type": "string", "description": "entity_id of the target (extracted or implied_N)"},
					"relation_type": map[string]any{"type": "string"},
					"confidence":    map[string]any{"type": "number"},
					"evidence":      map[string]any{"type": "string", "description": "The text pattern that implies this relation"},
				},
				"required": []string{"source_id", "target_id", "relation_type", "confidence", "evidence"},
			},
		},
	},
	"required": []string{"implied_entities", "implied_relations"},
}

func (p *Pipeline) inferImplied(ctx context.Context, document string, entities []llmextract.ExtractedEntity, relations []llmextract.EntityRelation) (*inferenceResult, error) {
	if len(entities) == 0 {
		return &inferenceResult{}, nil
	}

	// Build dynamic system prompt from allowed relation types
	allowed := llmextract.CollectAllowedRelations(p.matchConfigs)
	inferenceSystemPrompt := buildInferenceSystemPrompt(allowed)

	// Build entity summary including roles
	var entitySummary strings.Builder
	for _, e := range entities {
		roles := ""
		if len(e.Roles) > 0 {
			var roleNames []string
			for _, r := range e.Roles {
				roleNames = append(roleNames, r.Role)
			}
			roles = " [roles: " + strings.Join(roleNames, ", ") + "]"
		}
		fmt.Fprintf(&entitySummary, "- %s (%s): %s%s\n", e.EntityID, e.EntityType, string(e.Data), roles)
	}

	// Build relation summary
	var relationSummary strings.Builder
	for _, r := range relations {
		fmt.Fprintf(&relationSummary, "- %s --%s--> %s\n", r.SourceID, r.RelationType, r.TargetID)
	}
	if relationSummary.Len() == 0 {
		relationSummary.WriteString("(none identified yet)\n")
	}

	prompt := fmt.Sprintf(`Analyze this document for implicit information NOT already captured by the extracted entities and relations.

Extracted entities:
%s
Known relations:
%s
Document:
---
%s
---

Find:
1. Coreference resolutions (pronouns/possessives -> entities)
2. Implied entities NOT already in the extracted list
3. Implied relations NOT already in the known relations

For implied_relations, source_id and target_id MUST reference entity IDs from the extracted entities above OR implied entity IDs (implied_0, implied_1, etc.) that you define in implied_entities.
If no implicit information exists, return empty arrays.`, entitySummary.String(), relationSummary.String(), document)

	resp, err := genkit.Generate(ctx, p.g,
		ai.WithModelName(p.modelName),
		ai.WithSystem(inferenceSystemPrompt),
		ai.WithOutputSchema(inferenceOutputSchema),
		ai.WithPrompt(prompt),
		p.generationConfig(),
	)
	if err != nil {
		// Retry without strict schema
		resp, err = genkit.Generate(ctx, p.g,
			ai.WithModelName(p.modelName),
			ai.WithSystem(inferenceSystemPrompt),
			ai.WithPrompt(prompt+"\n\nOutput valid JSON with keys: implied_entities, implied_relations."),
			p.generationConfig(),
		)
		if err != nil {
			return nil, fmt.Errorf("LLM inference call: %w", err)
		}
	}

	p.collectUsage(resp, "infer_implied")

	var parsed struct {
		ImpliedEntities []struct {
			EntityType   string          `json:"entity_type"`
			Description  string          `json:"description"`
			Properties   json.RawMessage `json:"properties"`
			InferredFrom string          `json:"inferred_from"`
			Confidence   float64         `json:"confidence"`
		} `json:"implied_entities"`
		ImpliedRelations []struct {
			SourceID     string  `json:"source_id"`
			TargetID     string  `json:"target_id"`
			RelationType string  `json:"relation_type"`
			Confidence   float64 `json:"confidence"`
			Evidence     string  `json:"evidence"`
		} `json:"implied_relations"`
	}

	text := resp.Text()
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		// Try extracting JSON from response
		if jsonStr := extractJSON(text); jsonStr != "" {
			if err2 := json.Unmarshal([]byte(jsonStr), &parsed); err2 != nil {
				// Can't parse — return empty result rather than failing
				return &inferenceResult{}, nil
			}
		} else {
			return &inferenceResult{}, nil
		}
	}

	result := &inferenceResult{}

	// Build implied entities
	for i, ie := range parsed.ImpliedEntities {
		if ie.EntityType == "" || ie.Description == "" {
			continue
		}
		// Apply inference confidence penalty (20%)
		confidence := ie.Confidence * 0.8
		if confidence <= 0 {
			confidence = 0.5
		}

		result.ImpliedEntities = append(result.ImpliedEntities, llmextract.ImpliedEntity{
			EntityID:     fmt.Sprintf("implied_%d", i),
			EntityType:   ie.EntityType,
			Description:  ie.Description,
			Properties:   ie.Properties,
			InferredFrom: ie.InferredFrom,
			Confidence:   confidence,
			Lineage: llmextract.Lineage{
				SourceDocument: "input",
				ExtractedAt:    time.Now().UTC().Format(time.RFC3339),
				ModelID:        p.config.ModelName,
			},
		})
	}

	// Build implied relations
	for _, ir := range parsed.ImpliedRelations {
		if ir.SourceID == "" || ir.TargetID == "" || ir.RelationType == "" {
			continue
		}
		result.ImpliedRelations = append(result.ImpliedRelations, llmextract.EntityRelation{
			SourceID:     ir.SourceID,
			TargetID:     ir.TargetID,
			RelationType: ir.RelationType,
			Confidence:   defaultConfidence(ir.Confidence),
			Evidence:     ir.Evidence,
		})
	}

	return result, nil
}
