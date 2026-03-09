package pipeline

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/reflect/protoreflect"

	llmextract "github.com/laenen-partners/llmextract"
	"github.com/laenen-partners/llmextract/registry"
	"github.com/laenen-partners/llmextract/runner"
	"github.com/laenen-partners/llmextract/runner/direct"
	"github.com/laenen-partners/llmextract/validation"
)

// Pipeline orchestrates the entity extraction steps.
type Pipeline struct {
	runner       runner.StepRunner
	registry     *registry.Registry
	g            *genkit.Genkit
	config       llmextract.PipelineConfig
	modelName    string // fully qualified, e.g. "lmstudio/google/gemma-3-4b" or "googleai/gemini-2.5-flash"
	tools        []ai.ToolRef
	toolCalls    []llmextract.ToolCall          // collected during execution
	totalTokens  int                            // accumulated token usage across all LLM calls
	stepTokens   []llmextract.StepUsage         // per-step token usage
	stepTimeout  time.Duration                  // per-step timeout (0 = no timeout)
	matchConfigs *llmextract.MatchConfigRegistry // per-entity match configs for dynamic prompts
	validators   *validation.ValidatorRegistry   // per-entity semantic validators
	plugins      *validation.PluginRegistry      // post-extraction validation plugins
}

// Option configures a Pipeline.
type Option func(*Pipeline)

// WithRunner sets the step runner.
func WithRunner(r runner.StepRunner) Option {
	return func(p *Pipeline) { p.runner = r }
}

// WithModel overrides the default model name.
func WithModel(name string) Option {
	return func(p *Pipeline) { p.config.ModelName = name }
}

// WithMaxCorrectionRounds sets the maximum correction rounds.
func WithMaxCorrectionRounds(n int) Option {
	return func(p *Pipeline) { p.config.MaxCorrectionRounds = n }
}

// WithMinConfidence sets the minimum confidence threshold.
func WithMinConfidence(c float64) Option {
	return func(p *Pipeline) { p.config.MinConfidence = c }
}

// WithTemperature sets the sampling temperature. Lower values (0.0-0.2)
// produce more deterministic output, which is better for extraction tasks.
// Default is 0.1.
func WithTemperature(t float64) Option {
	return func(p *Pipeline) { p.config.Temperature = t }
}

// WithTopK limits sampling to the K most likely tokens at each step.
func WithTopK(k int) Option {
	return func(p *Pipeline) { p.config.TopK = k }
}

// WithTopP sets nucleus sampling probability. Limits sampling to tokens
// whose cumulative probability exceeds P.
func WithTopP(prob float64) Option {
	return func(p *Pipeline) { p.config.TopP = prob }
}

// WithMaxOutputTokens sets the maximum number of tokens per LLM response.
func WithMaxOutputTokens(n int) Option {
	return func(p *Pipeline) { p.config.MaxOutputTokens = n }
}

// WithTools sets the deterministic parsing tools available to the LLM.
func WithTools(tools []ai.ToolRef) Option {
	return func(p *Pipeline) { p.tools = tools }
}

// WithStepTimeout sets a per-step timeout for LLM calls.
func WithStepTimeout(d time.Duration) Option {
	return func(p *Pipeline) { p.stepTimeout = d }
}

// WithMatchConfigs sets the match config registry for dynamic relation prompts.
func WithMatchConfigs(mcr *llmextract.MatchConfigRegistry) Option {
	return func(p *Pipeline) { p.matchConfigs = mcr }
}

// WithValidators sets the validator registry for semantic validation.
func WithValidators(vr *validation.ValidatorRegistry) Option {
	return func(p *Pipeline) { p.validators = vr }
}

// WithPlugins sets the plugin registry for post-extraction validation.
func WithPlugins(pr *validation.PluginRegistry) Option {
	return func(p *Pipeline) { p.plugins = pr }
}

// New creates a new Pipeline.
func New(g *genkit.Genkit, reg *registry.Registry, opts ...Option) *Pipeline {
	p := &Pipeline{
		runner:   direct.New(),
		registry: reg,
		g:        g,
		config:   llmextract.DefaultPipelineConfig(),
	}
	for _, opt := range opts {
		opt(p)
	}
	p.modelName = p.config.ModelName
	return p
}

// stepCtx returns a context with a timeout if stepTimeout is configured.
func (p *Pipeline) stepCtx(ctx context.Context) (context.Context, context.CancelFunc) {
	if p.stepTimeout > 0 {
		return context.WithTimeout(ctx, p.stepTimeout)
	}
	return ctx, func() {}
}

// Extract runs the full entity extraction pipeline on a markdown document.
func (p *Pipeline) Extract(ctx context.Context, document string) (*llmextract.ExtractionOutput, error) {
	start := time.Now()
	docID := randomID()
	p.toolCalls = nil // reset for this extraction
	p.totalTokens = 0
	p.stepTokens = nil

	// Step 1: Discover relevant entity types
	slog.Info("pipeline step starting", "step", "discover_entities")
	sCtx, cancel := p.stepCtx(ctx)
	discovery, err := runner.RunStep(sCtx, p.runner, "discover_entities", func(ctx context.Context) (*llmextract.DiscoveryResult, error) {
		return p.discoverEntities(ctx, document)
	})
	cancel()
	if err != nil {
		return nil, fmt.Errorf("discovery: %w", err)
	}
	slog.Info("pipeline step done", "step", "discover_entities", "entity_types", len(discovery.RelevantEntities))

	// Step 2: Extract entities (one sub-step per entity type)
	var allEntities []llmextract.ExtractedEntity
	for _, re := range discovery.RelevantEntities {
		stepName := "extract_" + re.EntityType
		slog.Info("pipeline step starting", "step", stepName)
		sCtx, cancel := p.stepCtx(ctx)
		entities, err := runner.RunStep(sCtx, p.runner, stepName, func(ctx context.Context) ([]llmextract.ExtractedEntity, error) {
			return p.extractEntities(ctx, document, re)
		})
		cancel()
		if err != nil {
			return nil, fmt.Errorf("extraction of %s: %w", re.EntityType, err)
		}
		slog.Info("pipeline step done", "step", stepName, "count", len(entities))
		allEntities = append(allEntities, entities...)
	}

	// Step 3: Validate and correct
	slog.Info("pipeline step starting", "step", "validate_and_correct")
	sCtx, cancel = p.stepCtx(ctx)
	correctionResult, err := runner.RunStep(sCtx, p.runner, "validate_and_correct", func(ctx context.Context) (*correctionOutput, error) {
		return p.validateAndCorrect(ctx, document, allEntities)
	})
	cancel()
	if err != nil {
		return nil, fmt.Errorf("correction: %w", err)
	}
	slog.Info("pipeline step done", "step", "validate_and_correct")

	// Step 4: Resolve intra-document entity relations
	slog.Info("pipeline step starting", "step", "resolve_relations")
	sCtx, cancel = p.stepCtx(ctx)
	resolved, err := runner.RunStep(sCtx, p.runner, "resolve_relations", func(ctx context.Context) (*relationResult, error) {
		return p.resolveRelations(ctx, document, correctionResult.Entities)
	})
	cancel()
	if err != nil {
		return nil, fmt.Errorf("relation resolution: %w", err)
	}
	slog.Info("pipeline step done", "step", "resolve_relations")

	// Step 4b: Contextual inference (implied entities and relations)
	slog.Info("pipeline step starting", "step", "infer_implied")
	sCtx, cancel = p.stepCtx(ctx)
	inferred, err := runner.RunStep(sCtx, p.runner, "infer_implied", func(ctx context.Context) (*inferenceResult, error) {
		return p.inferImplied(ctx, document, resolved.Entities, resolved.Relations)
	})
	cancel()
	if err != nil {
		return nil, fmt.Errorf("inference: %w", err)
	}
	slog.Info("pipeline step done", "step", "infer_implied",
		"implied_entities", len(inferred.ImpliedEntities),
		"implied_relations", len(inferred.ImpliedRelations))

	// Step 5: Score confidence
	slog.Info("pipeline step starting", "step", "score_confidence")
	scored, err := runner.RunStep(ctx, p.runner, "score_confidence", func(ctx context.Context) ([]llmextract.ExtractedEntity, error) {
		return p.scoreConfidence(correctionResult.Corrections, resolved.Entities)
	})
	if err != nil {
		return nil, fmt.Errorf("scoring: %w", err)
	}
	slog.Info("pipeline step done", "step", "score_confidence")

	// Build output
	overall := computeOverallConfidence(scored)
	return &llmextract.ExtractionOutput{
		DocumentID:        docID,
		DocumentType:      inferDocumentType(discovery),
		Entities:          scored,
		Relations:         resolved.Relations,
		ImpliedEntities:   inferred.ImpliedEntities,
		ImpliedRelations:  inferred.ImpliedRelations,
		DiscoveryTrace:    *discovery,
		CorrectionLog:     correctionResult.Corrections,
		OverallConfidence: overall,
		ProcessingMeta: llmextract.ProcessingMeta{
			ModelID:         p.config.ModelName,
			TotalRounds:     correctionResult.TotalRounds,
			TotalTokensUsed: p.totalTokens,
			StepTokens:      p.stepTokens,
			DurationMs:      time.Since(start).Milliseconds(),
			ToolCalls:       p.toolCalls,
		},
	}, nil
}

// generationConfig returns the ai.WithConfig option for LLM calls based on
// pipeline config. Uses map[string]any so it works with all Genkit model
// plugins (including compat_oai which doesn't accept GenerationCommonConfig).
func (p *Pipeline) generationConfig() ai.GenerateOption {
	cfg := map[string]any{
		"temperature": p.config.Temperature,
	}
	if p.config.TopK > 0 {
		cfg["topK"] = p.config.TopK
	}
	if p.config.TopP > 0 {
		cfg["topP"] = p.config.TopP
	}
	if p.config.MaxOutputTokens > 0 {
		cfg["maxOutputTokens"] = p.config.MaxOutputTokens
	}
	return ai.WithConfig(cfg)
}

// normalizeEntityData round-trips entity data through protojson to strip unknown
// fields (e.g. source_text, title) that the LLM may have injected into the data object.
func (p *Pipeline) normalizeEntityData(entityType protoreflect.FullName, data json.RawMessage) (json.RawMessage, error) {
	msg, err := p.registry.NewInstance(entityType)
	if err != nil {
		return nil, err
	}
	if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(data, msg); err != nil {
		return nil, err
	}
	return protojson.Marshal(msg)
}

// collectUsage accumulates token usage from a Genkit response for the given step.
func (p *Pipeline) collectUsage(resp *ai.ModelResponse, step string) {
	if resp == nil || resp.Usage == nil {
		return
	}
	total := resp.Usage.TotalTokens
	if total == 0 {
		total = resp.Usage.InputTokens + resp.Usage.OutputTokens
	}
	p.totalTokens += total
	p.stepTokens = append(p.stepTokens, llmextract.StepUsage{
		Step:         step,
		InputTokens:  resp.Usage.InputTokens,
		OutputTokens: resp.Usage.OutputTokens,
		TotalTokens:  total,
	})
}

// collectToolCalls extracts tool usage from a Genkit response's message history.
func (p *Pipeline) collectToolCalls(resp *ai.ModelResponse, step string) {
	if resp == nil {
		return
	}
	for _, msg := range resp.History() {
		for _, part := range msg.Content {
			if part.IsToolRequest() {
				p.toolCalls = append(p.toolCalls, llmextract.ToolCall{
					Step:  step,
					Tool:  part.ToolRequest.Name,
					Input: part.ToolRequest.Input,
				})
			}
			if part.IsToolResponse() {
				// Match with the last tool call of the same name to add the output
				for i := len(p.toolCalls) - 1; i >= 0; i-- {
					if p.toolCalls[i].Tool == part.ToolResponse.Name && p.toolCalls[i].Output == nil {
						p.toolCalls[i].Output = part.ToolResponse.Output
						break
					}
				}
			}
		}
	}
}

func randomID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func inferDocumentType(d *llmextract.DiscoveryResult) string {
	if len(d.RelevantEntities) == 0 {
		return "unknown"
	}
	// Use the highest-confidence entity type as a hint for document type.
	best := d.RelevantEntities[0]
	for _, re := range d.RelevantEntities[1:] {
		if re.Confidence > best.Confidence {
			best = re
		}
	}
	return best.EntityType
}

func computeOverallConfidence(entities []llmextract.ExtractedEntity) float64 {
	if len(entities) == 0 {
		return 0
	}
	var sum float64
	count := 0
	for _, e := range entities {
		if e.MergedInto == "" { // only count canonical entities
			sum += e.Confidence
			count++
		}
	}
	if count == 0 {
		return 0
	}
	return sum / float64(count)
}
