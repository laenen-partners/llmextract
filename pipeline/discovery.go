package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/firebase/genkit/go/ai"

	llmextract "github.com/laenen-partners/llmextract"
)

const discoverySystemPrompt = `You are an entity extraction system. Your job is to analyze a document and determine which entity types are present and relevant for extraction.

You will be given:
1. A list of available entity types with their JSON schemas
2. A document in markdown format

For each entity type that is present in the document, provide:
- The fully qualified entity type name (exactly as shown)
- Your reasoning for why this entity type is relevant
- A confidence score between 0.0 and 1.0

Only include entity types that are actually present in the document. Do not include types that are not relevant.`

var discoveryOutputSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"relevant_entities": map[string]any{
			"type": "array",
			"items": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"entity_type": map[string]any{"type": "string", "description": "Fully qualified proto name, e.g. entities.v1.Person"},
					"reasoning":   map[string]any{"type": "string", "description": "Why this entity type is relevant for this document"},
					"confidence":  map[string]any{"type": "number", "description": "Confidence score 0.0-1.0"},
				},
				"required": []string{"entity_type", "reasoning", "confidence"},
			},
		},
	},
	"required": []string{"relevant_entities"},
}

func (p *Pipeline) discoverEntities(ctx context.Context, document string) (*llmextract.DiscoveryResult, error) {
	schemasSummary := p.registry.SchemasSummary()

	prompt := fmt.Sprintf(`Available entity types:

%s

Document:
---
%s
---

Identify which entity types are present in this document.`, schemasSummary, document)

	resp, err := p.generate(ctx,
		ai.WithModelName(p.modelName),
		ai.WithSystem(discoverySystemPrompt),
		ai.WithOutputSchema(discoveryOutputSchema),
		ai.WithPrompt(prompt),
		p.generationConfig(),
	)
	if err != nil {
		// Retry without strict schema enforcement for models that struggle with it
		resp, err = p.generate(ctx,
			ai.WithModelName(p.modelName),
			ai.WithSystem(discoverySystemPrompt),
			ai.WithPrompt(prompt+"\n\nRespond with JSON: {\"relevant_entities\": [{\"entity_type\": \"...\", \"reasoning\": \"...\", \"confidence\": 0.9}]}"),
			p.generationConfig(),
		)
		if err != nil {
			return nil, fmt.Errorf("LLM discovery call: %w", err)
		}
	}

	p.collectUsage(resp, "discover_entities")

	var result llmextract.DiscoveryResult
	if err := json.Unmarshal([]byte(resp.Text()), &result); err != nil {
		// Try to extract JSON from the response text (model may have added extra text)
		if extracted := extractJSON(resp.Text()); extracted != "" {
			if err2 := json.Unmarshal([]byte(extracted), &result); err2 == nil {
				goto normalize
			}
		}
		return nil, fmt.Errorf("parsing discovery response: %w", err)
	}

normalize:
	// Normalize entity type names: models may return "Person" instead of "entities.v1.Person"
	p.normalizeEntityTypes(&result)

	return &result, nil
}

// normalizeEntityTypes fixes entity type names that don't match the registry.
// Models may return short names like "Person" instead of "entities.v1.Person".
func (p *Pipeline) normalizeEntityTypes(result *llmextract.DiscoveryResult) {
	allTypes := p.registry.AllTypes()
	filtered := result.RelevantEntities[:0]
	for _, re := range result.RelevantEntities {
		// Check if the entity type exists as-is
		if p.registry.HasType(re.EntityType) {
			filtered = append(filtered, re)
			continue
		}
		// Try fuzzy matching: find a registered type whose short name matches
		matched := false
		for _, fullName := range allTypes {
			short := shortEntityName(string(fullName))
			if strings.EqualFold(re.EntityType, short) || strings.EqualFold(re.EntityType, string(fullName)) {
				re.EntityType = string(fullName)
				filtered = append(filtered, re)
				matched = true
				break
			}
		}
		if !matched {
			slog.Warn("unknown entity type from discovery, skipping", "entity_type", re.EntityType)
		}
	}
	result.RelevantEntities = filtered
}

// extractJSON tries to find a JSON object in text that may contain extra content.
func extractJSON(text string) string {
	start := strings.Index(text, "{")
	if start == -1 {
		return ""
	}
	// Find the matching closing brace
	depth := 0
	for i := start; i < len(text); i++ {
		switch text[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return text[start : i+1]
			}
		}
	}
	return ""
}
