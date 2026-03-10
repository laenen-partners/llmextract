package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/firebase/genkit/go/ai"

	llmextract "github.com/laenen-partners/llmextract"
)

type relationResult struct {
	Entities  []llmextract.ExtractedEntity `json:"entities"`
	Relations []llmextract.EntityRelation  `json:"relations"`
}

// relationTypeDescriptions maps every known relation type to a short description
// used when building the system prompt for relation resolution and inference.
var relationTypeDescriptions = map[string]string{
	"same_as":         "Two entities refer to the same real-world thing (must be same entity type)",
	"affiliated_with": "Person belongs to / works for an organization",
	"sender_of":       "Entity is the sender of a document/communication",
	"recipient_of":    "Entity is the recipient",
	"issuer_of":       "Organization/person issued a document",
	"buyer_on":        "Buyer/customer on a commercial document",
	"addressed_at":    "An address belongs to an entity",
	"refers_to":       "Generic reference between entities",
	"part_of":         "Entity is part of another",
	"attached_to":     "A document/section is attached to another",
	"parent_of":       "Entity is a parent of another (family)",
	"child_of":        "Entity is a child of another (family)",
	"spouse_of":       "Entities are married or partners (family)",
	"sibling_of":      "Entities are siblings (family)",
	"participant_in":  "Entity participates in an event",
	"honoree_of":      "Event is in honor of an entity",
	"organized_by":    "Entity organized an event",
}

// buildRelationSystemPrompt constructs the relation system prompt using only
// the provided allowed types. If allowedTypes is empty, all types are included.
func buildRelationSystemPrompt(allowedTypes []string) string {
	types := allowedTypes
	if len(types) == 0 {
		types = make([]string, 0, len(relationTypeDescriptions))
		for t := range relationTypeDescriptions {
			types = append(types, t)
		}
		sort.Strings(types)
	}

	var b strings.Builder
	b.WriteString(`You are an entity relationship analysis system. Given a set of extracted entities from a document, you must:

1. Identify which entities refer to the same real-world thing (same_as relations)
2. Identify semantic relationships between entities

Relation types:
`)
	for _, t := range types {
		desc, ok := relationTypeDescriptions[t]
		if !ok {
			desc = t
		}
		fmt.Fprintf(&b, "- %s: %s\n", t, desc)
	}
	return b.String()
}

var relationOutputSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"same_as_pairs": map[string]any{
			"type": "array",
			"items": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"source_id":  map[string]any{"type": "string"},
					"target_id":  map[string]any{"type": "string"},
					"reasoning":  map[string]any{"type": "string", "description": "Why these two entities are the same"},
					"confidence": map[string]any{"type": "number", "description": "Confidence score between 0.0 and 1.0, e.g. 0.90"},
				},
				"required": []string{"source_id", "target_id", "reasoning", "confidence"},
			},
		},
		"relations": map[string]any{
			"type": "array",
			"items": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"source_id":     map[string]any{"type": "string"},
					"target_id":     map[string]any{"type": "string"},
					"relation_type": map[string]any{"type": "string"},
					"confidence":    map[string]any{"type": "number", "description": "Confidence score between 0.0 and 1.0, e.g. 0.85"},
					"evidence":      map[string]any{"type": "string", "description": "Quote or brief explanation from the document supporting this relation"},
				},
				"required": []string{"source_id", "target_id", "relation_type", "confidence", "evidence"},
			},
		},
		"roles": map[string]any{
			"type": "array",
			"items": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"entity_id":  map[string]any{"type": "string"},
					"role":       map[string]any{"type": "string"},
					"context":    map[string]any{"type": "string"},
					"confidence": map[string]any{"type": "number"},
				},
				"required": []string{"entity_id", "role", "confidence"},
			},
		},
	},
	"required": []string{"same_as_pairs", "relations", "roles"},
}

func (p *Pipeline) resolveRelations(ctx context.Context, document string, entities []llmextract.ExtractedEntity) (*relationResult, error) {
	if len(entities) < 2 {
		return &relationResult{Entities: entities}, nil
	}

	// Build dynamic system prompt from allowed relation types
	allowed := llmextract.CollectAllowedRelations(p.matchConfigs)
	systemPrompt := buildRelationSystemPrompt(allowed)

	// Build entity summary — list IDs clearly and separately from data.
	validIDs := make(map[string]bool, len(entities))
	var summary strings.Builder
	var idList strings.Builder
	for i, e := range entities {
		validIDs[e.EntityID] = true
		fmt.Fprintf(&summary, "- %s (%s): %s\n", e.EntityID, e.EntityType, string(e.Data))
		if i > 0 {
			idList.WriteString(", ")
		}
		fmt.Fprintf(&idList, "%q", e.EntityID)
	}

	prompt := fmt.Sprintf(`Given these extracted entities from a single document, identify:
1. Which entities refer to the same real-world thing (same_as pairs)
2. What roles each entity plays (sender, recipient, buyer, issuer, etc.)
3. What relationships exist between entities

IMPORTANT: The ONLY valid entity IDs are: [%s]
Every source_id, target_id, and entity_id in your response MUST be one of these exact strings.
Do NOT use entity names, types, or any other values as IDs.

Entities:
%s

Document:
---
%s
---

Respond with JSON matching the schema. For every relation and same_as pair:
- source_id and target_id MUST be from [%s]
- confidence MUST be a number between 0.0 and 1.0 (e.g. 0.85, 0.92)
- evidence MUST be a brief quote or explanation from the document`,
		idList.String(), summary.String(), document, idList.String())

	resp, err := p.generate(ctx,
		ai.WithModelName(p.modelName),
		ai.WithSystem(systemPrompt),
		ai.WithOutputSchema(relationOutputSchema),
		ai.WithPrompt(prompt),
		p.generationConfig(),
	)
	if err != nil {
		// If schema validation fails, retry without strict schema enforcement
		resp, err = p.generate(ctx,
			ai.WithModelName(p.modelName),
			ai.WithSystem(systemPrompt),
			ai.WithPrompt(prompt+"\n\nOutput valid JSON with keys: same_as_pairs, relations, roles."),
			p.generationConfig(),
		)
		if err != nil {
			return nil, fmt.Errorf("LLM relation resolution call: %w", err)
		}
	}

	p.collectUsage(resp, "resolve_relations")

	var parsed struct {
		SameAsPairs []struct {
			SourceID   string  `json:"source_id"`
			TargetID   string  `json:"target_id"`
			Reasoning  string  `json:"reasoning"`
			Confidence float64 `json:"confidence"`
		} `json:"same_as_pairs"`
		Relations []struct {
			SourceID     string  `json:"source_id"`
			TargetID     string  `json:"target_id"`
			RelationType string  `json:"relation_type"`
			Confidence   float64 `json:"confidence"`
			Evidence     string  `json:"evidence"`
		} `json:"relations"`
		Roles []struct {
			EntityID   string  `json:"entity_id"`
			Role       string  `json:"role"`
			Context    string  `json:"context"`
			Confidence float64 `json:"confidence"`
		} `json:"roles"`
	}
	responseText := resp.Text()

	// Local models often wrap JSON in markdown code blocks — extract the JSON object.
	if cleaned := extractJSON(responseText); cleaned != "" {
		responseText = cleaned
	}

	if err := json.Unmarshal([]byte(responseText), &parsed); err != nil {
		// LLMs sometimes return roles as an object instead of an array.
		// Retry with a lenient struct that accepts roles as raw JSON.
		var lenient struct {
			SameAsPairs json.RawMessage `json:"same_as_pairs"`
			Relations   json.RawMessage `json:"relations"`
			Roles       json.RawMessage `json:"roles"`
		}
		if err2 := json.Unmarshal([]byte(responseText), &lenient); err2 == nil {
			// Re-parse only the fields that are valid arrays.
			if lenient.SameAsPairs != nil {
				_ = json.Unmarshal(lenient.SameAsPairs, &parsed.SameAsPairs)
			}
			if lenient.Relations != nil {
				_ = json.Unmarshal(lenient.Relations, &parsed.Relations)
			}
			if lenient.Roles != nil {
				// Try array first; if it fails (object/string), wrap in array.
				if err3 := json.Unmarshal(lenient.Roles, &parsed.Roles); err3 != nil {
					var single struct {
						EntityID   string  `json:"entity_id"`
						Role       string  `json:"role"`
						Context    string  `json:"context"`
						Confidence float64 `json:"confidence"`
					}
					if json.Unmarshal(lenient.Roles, &single) == nil && single.EntityID != "" {
						parsed.Roles = append(parsed.Roles, single)
					}
				}
			}
		} else {
			slog.Warn("relation JSON parse failed", "error", err)
			return &relationResult{Entities: entities}, nil
		}
	}

	// Apply same_as merges
	result := &relationResult{
		Entities: make([]llmextract.ExtractedEntity, len(entities)),
	}
	copy(result.Entities, entities)

	// Build index
	idx := make(map[string]int)
	for i, e := range result.Entities {
		idx[e.EntityID] = i
	}

	// Add same_as relations and perform merges (skip entries with invalid IDs)
	var droppedIDs int
	for _, pair := range parsed.SameAsPairs {
		if pair.SourceID == "" || pair.TargetID == "" || pair.SourceID == pair.TargetID {
			continue
		}
		if !validIDs[pair.SourceID] || !validIDs[pair.TargetID] {
			droppedIDs++
			continue
		}

		si, sok := idx[pair.SourceID]
		ti, tok := idx[pair.TargetID]
		if sok && tok && !isSameEntity(result.Entities[si], result.Entities[ti]) {
			slog.Warn("rejected same_as: entities have different identifying data",
				"source", pair.SourceID, "target", pair.TargetID)
			continue
		}

		result.Relations = append(result.Relations, llmextract.EntityRelation{
			SourceID:     pair.SourceID,
			TargetID:     pair.TargetID,
			RelationType: "same_as",
			Confidence:   defaultConfidence(pair.Confidence),
			Evidence:     pair.Reasoning,
		})

		if sok && tok {
			result.Entities[si].MergedInto = pair.TargetID
			result.Entities[ti].MergedFrom = append(result.Entities[ti].MergedFrom, pair.SourceID)
		}
	}

	// Add semantic relations (skip entries with invalid IDs)
	for _, rel := range parsed.Relations {
		if rel.SourceID == "" || rel.TargetID == "" || rel.RelationType == "" {
			continue
		}
		if !validIDs[rel.SourceID] || !validIDs[rel.TargetID] {
			droppedIDs++
			continue
		}
		result.Relations = append(result.Relations, llmextract.EntityRelation{
			SourceID:     rel.SourceID,
			TargetID:     rel.TargetID,
			RelationType: rel.RelationType,
			Confidence:   defaultConfidence(rel.Confidence),
			Evidence:     rel.Evidence,
		})
	}

	if droppedIDs > 0 {
		slog.Warn("dropped relations with invalid entity IDs (LLM used names instead of IDs)", "count", droppedIDs)
	}

	// Apply roles (skip incomplete entries)
	for _, role := range parsed.Roles {
		if role.EntityID == "" || role.Role == "" {
			continue
		}
		if i, ok := idx[role.EntityID]; ok {
			result.Entities[i].Roles = append(result.Entities[i].Roles, llmextract.EntityRole{
				Role:       role.Role,
				Context:    role.Context,
				Confidence: defaultConfidence(role.Confidence),
			})
		}
	}

	return result, nil
}

// isSameEntity checks whether two entities plausibly refer to the same
// real-world thing by comparing their string field values. Entities of
// different types are never the same. For entities of the same type, we
// compare all non-empty string values — if any shared key has a different
// value, the entities are considered distinct.
func isSameEntity(a, b llmextract.ExtractedEntity) bool {
	if a.EntityType != b.EntityType {
		return false
	}
	ma := jsonStringFields(a.Data)
	mb := jsonStringFields(b.Data)
	if len(ma) == 0 || len(mb) == 0 {
		return true // can't compare — allow the LLM's judgement
	}
	for k, va := range ma {
		if vb, ok := mb[k]; ok && va != "" && vb != "" {
			if !strings.EqualFold(va, vb) {
				return false
			}
		}
	}
	return true
}

// jsonStringFields extracts top-level string key-value pairs from a JSON object.
func jsonStringFields(data json.RawMessage) map[string]string {
	var m map[string]json.RawMessage
	if json.Unmarshal(data, &m) != nil {
		return nil
	}
	result := make(map[string]string, len(m))
	for k, v := range m {
		var s string
		if json.Unmarshal(v, &s) == nil {
			result[k] = s
		}
	}
	return result
}

// defaultConfidence returns a sensible fallback when the LLM omits confidence
// or returns 0. Small models frequently skip numeric fields.
func defaultConfidence(c float64) float64 {
	if c > 0 {
		return c
	}
	return 0.70
}
