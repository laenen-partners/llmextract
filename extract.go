package llmextract

import "encoding/json"

// ExtractionOutput is the top-level result of the entity extraction pipeline.
type ExtractionOutput struct {
	DocumentID        string             `json:"document_id"`
	DocumentType      string             `json:"document_type"`
	Entities          []ExtractedEntity  `json:"entities"`
	Relations         []EntityRelation   `json:"relations"`
	ImpliedEntities   []ImpliedEntity    `json:"implied_entities,omitempty"`
	ImpliedRelations  []EntityRelation   `json:"implied_relations,omitempty"`
	DiscoveryTrace    DiscoveryResult    `json:"discovery_trace"`
	CorrectionLog     []CorrectionEntry  `json:"correction_log"`
	OverallConfidence float64            `json:"overall_confidence"`
	ProcessingMeta    ProcessingMeta     `json:"processing_meta"`
}

// ExtractedEntity represents a single entity extracted from the document.
type ExtractedEntity struct {
	EntityID   string          `json:"entity_id"`
	EntityType string          `json:"entity_type"` // fully qualified proto name, e.g. "entities.v1.Person"
	Data       json.RawMessage `json:"data"`
	Confidence float64         `json:"confidence"`
	Lineage    Lineage         `json:"lineage"`
	Roles      []EntityRole    `json:"roles,omitempty"`
	MergedInto string          `json:"merged_into,omitempty"`
	MergedFrom []string        `json:"merged_from,omitempty"`
}

// ImpliedEntity represents an entity discovered via contextual inference rather
// than direct extraction. Unlike ExtractedEntity, implied entities are not
// constrained to registered proto types — they use free-form types and properties.
type ImpliedEntity struct {
	EntityID     string          `json:"entity_id"`      // e.g. "implied_0"
	EntityType   string          `json:"entity_type"`    // free-form: "event", "group", "person"
	Description  string          `json:"description"`    // human-readable: "birthday party in Dubai"
	Properties   json.RawMessage `json:"properties"`     // key-value pairs from context
	InferredFrom string          `json:"inferred_from"`  // source text that implied this entity
	Confidence   float64         `json:"confidence"`
	Lineage      Lineage         `json:"lineage"`
}

// EntityRelation captures a directed relationship between two extracted entities.
type EntityRelation struct {
	SourceID     string  `json:"source_id"`
	TargetID     string  `json:"target_id"`
	RelationType string  `json:"relation_type"`
	Confidence   float64 `json:"confidence"`
	Evidence     string  `json:"evidence"`
}

// EntityRole describes a role an entity plays in a specific context.
type EntityRole struct {
	Role       string  `json:"role"`
	Context    string  `json:"context"`
	Confidence float64 `json:"confidence"`
}

// Lineage tracks where an extracted value came from.
type Lineage struct {
	SourceDocument  string   `json:"source_document"`
	SourceSections  []string `json:"source_sections,omitempty"`
	SourceSpans     []Span   `json:"source_spans,omitempty"`
	ExtractedAt     string   `json:"extracted_at"`
	ModelID         string   `json:"model_id"`
	FlowTraceID     string   `json:"flow_trace_id,omitempty"`
	CorrectionRound int      `json:"correction_round"`
}

// Span identifies a character range in the source document.
type Span struct {
	Start int    `json:"start"`
	End   int    `json:"end"`
	Text  string `json:"text"`
}

// DiscoveryResult lists which entity types the LLM considers relevant.
type DiscoveryResult struct {
	RelevantEntities []RelevantEntity `json:"relevant_entities"`
}

// RelevantEntity is an entity type identified as relevant during discovery.
type RelevantEntity struct {
	EntityType string  `json:"entity_type"` // fully qualified proto name
	Reasoning  string  `json:"reasoning"`
	Confidence float64 `json:"confidence"`
}

// CorrectionEntry records a single field correction made during the validation loop.
type CorrectionEntry struct {
	Round    int             `json:"round"`
	EntityID string          `json:"entity_id"`
	Field    string          `json:"field"`
	OldValue json.RawMessage `json:"old_value"`
	NewValue json.RawMessage `json:"new_value"`
	Reason   string          `json:"reason"`
}

// ProcessingMeta contains metadata about the extraction run.
type ProcessingMeta struct {
	ModelID         string      `json:"model_id"`
	TotalRounds     int         `json:"total_rounds"`
	TotalTokensUsed int         `json:"total_tokens_used"`
	StepTokens      []StepUsage `json:"step_tokens,omitempty"`
	FlowTraceID     string      `json:"flow_trace_id,omitempty"`
	DurationMs      int64       `json:"duration_ms"`
	ToolCalls       []ToolCall  `json:"tool_calls,omitempty"`
}

// StepUsage records token consumption for a single pipeline step.
type StepUsage struct {
	Step         string `json:"step"`
	InputTokens  int    `json:"input_tokens"`
	OutputTokens int    `json:"output_tokens"`
	TotalTokens  int    `json:"total_tokens"`
}

// ToolCall records a single tool invocation during extraction.
type ToolCall struct {
	Step   string `json:"step"`   // pipeline step where the call happened
	Tool   string `json:"tool"`   // tool name, e.g. "parse_money"
	Input  any    `json:"input"`  // tool input
	Output any    `json:"output"` // tool result
}

// PipelineConfig holds configuration for the extraction pipeline.
type PipelineConfig struct {
	ModelName           string
	MaxCorrectionRounds int
	MinConfidence       float64
	Temperature         float64 // 0 = use default (0.1)
	TopK                int     // 0 = provider default
	TopP                float64 // 0 = provider default
	MaxOutputTokens     int     // 0 = provider default
}

// DefaultPipelineConfig returns sensible defaults.
func DefaultPipelineConfig() PipelineConfig {
	return PipelineConfig{
		ModelName:           "googleai/gemini-2.5-flash",
		MaxCorrectionRounds: 3,
		MinConfidence:       0.6,
		Temperature:         0.1,
	}
}
