package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"
	"google.golang.org/protobuf/reflect/protoreflect"

	llmextract "github.com/laenen-partners/llmextract"
)

const extractionSystemPrompt = `You are a precise entity extraction system. Extract all instances of the specified entity type from the document.

Rules:
- Extract ONLY the specified entity type
- Extract ALL instances found in the document
- Use the exact field names from the schema
- For dates, use ISO 8601 format (YYYY-MM-DDTHH:MM:SSZ)
- For monetary amounts, use numbers (not strings)
- If a field value is not present in the document, omit it
- For each entity, provide a confidence score (0.0-1.0) reflecting how certain you are about the extraction
- Include source_text: the literal text span from the document that this entity was extracted from`

func (p *Pipeline) extractEntities(ctx context.Context, document string, re llmextract.RelevantEntity) ([]llmextract.ExtractedEntity, error) {
	entityType := protoreflect.FullName(re.EntityType)
	entitySchema, ok := p.registry.Schema(entityType)
	if !ok {
		return nil, fmt.Errorf("schema not found for %s", entityType)
	}

	schemaJSON, _ := json.MarshalIndent(entitySchema, "", "  ")

	outputSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"entities": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"data":        entitySchema,
						"confidence":  map[string]any{"type": "number", "description": "Confidence 0.0-1.0"},
						"source_text": map[string]any{"type": "string", "description": "The literal text span this entity was extracted from"},
					},
					"required": []string{"data", "confidence"},
				},
			},
		},
		"required": []string{"entities"},
	}

	prompt := fmt.Sprintf(`Extract all instances of type **%s** from the document below.

Schema:
%s

Document:
---
%s
---`, re.EntityType, string(schemaJSON), document)

	generateOpts := []ai.GenerateOption{
		ai.WithModelName(p.modelName),
		ai.WithSystem(extractionSystemPrompt),
		ai.WithOutputSchema(outputSchema),
		ai.WithPrompt(prompt),
	}
	if len(p.tools) > 0 {
		generateOpts = append(generateOpts, ai.WithTools(p.tools...), ai.WithMaxTurns(2))
	}
	resp, err := genkit.Generate(ctx, p.g, generateOpts...)
	if err != nil {
		// Retry without strict schema for models that struggle with structured output
		fallbackOpts := []ai.GenerateOption{
			ai.WithModelName(p.modelName),
			ai.WithSystem(extractionSystemPrompt),
			ai.WithPrompt(prompt + "\n\nRespond with JSON: {\"entities\": [{\"data\": {...}, \"confidence\": 0.9, \"source_text\": \"...\"}]}"),
		}
		if len(p.tools) > 0 {
			fallbackOpts = append(fallbackOpts, ai.WithTools(p.tools...), ai.WithMaxTurns(2))
		}
		resp, err = genkit.Generate(ctx, p.g, fallbackOpts...)
		if err != nil {
			return nil, fmt.Errorf("LLM extraction call for %s: %w", re.EntityType, err)
		}
	}
	p.collectUsage(resp, "extract_"+re.EntityType)
	p.collectToolCalls(resp, "extract_"+re.EntityType)

	var extracted struct {
		Entities []struct {
			Data       json.RawMessage `json:"data"`
			Confidence float64         `json:"confidence"`
			SourceText string          `json:"source_text"`
		} `json:"entities"`
	}
	text := resp.Text()
	if err := json.Unmarshal([]byte(text), &extracted); err != nil {
		// Try extracting JSON from response text
		if jsonStr := extractJSON(text); jsonStr != "" {
			if err2 := json.Unmarshal([]byte(jsonStr), &extracted); err2 != nil {
				return nil, fmt.Errorf("parsing extraction response for %s: %w", re.EntityType, err)
			}
		} else {
			return nil, fmt.Errorf("parsing extraction response for %s: %w", re.EntityType, err)
		}
	}

	// If entities have nil Data (model put fields directly instead of under "data"), try to recover
	if len(extracted.Entities) > 0 && extracted.Entities[0].Data == nil {
		// Try alternative parse: entities are the data objects directly
		var altExtracted struct {
			Entities []json.RawMessage `json:"entities"`
		}
		if err := json.Unmarshal([]byte(text), &altExtracted); err == nil {
			for idx, raw := range altExtracted.Entities {
				if idx < len(extracted.Entities) {
					extracted.Entities[idx].Data = raw
					if extracted.Entities[idx].Confidence == 0 {
						extracted.Entities[idx].Confidence = 0.5 // default confidence for recovered entities
					}
				}
			}
		}
	}

	// Build short type prefix for entity IDs (e.g., "person", "invoice")
	shortName := shortEntityName(re.EntityType)

	var result []llmextract.ExtractedEntity
	for i, e := range extracted.Entities {
		entity := llmextract.ExtractedEntity{
			EntityID:   fmt.Sprintf("%s_%d", shortName, i),
			EntityType: re.EntityType,
			Data:       e.Data,
			Confidence: e.Confidence,
			Lineage: llmextract.Lineage{
				SourceDocument:  "input",
				ExtractedAt:     time.Now().UTC().Format(time.RFC3339),
				ModelID:         p.config.ModelName,
				CorrectionRound: 0,
			},
		}
		if e.SourceText != "" {
			entity.Lineage.SourceSpans = []llmextract.Span{
				{Text: e.SourceText},
			}
		}
		result = append(result, entity)
	}

	return result, nil
}

// shortEntityName extracts a short name from a fully qualified proto name.
// e.g., "entities.v1.Person" -> "person"
func shortEntityName(fullName string) string {
	parts := strings.Split(fullName, ".")
	name := parts[len(parts)-1]
	return strings.ToLower(name)
}
