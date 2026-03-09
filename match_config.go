package llmextract

import (
	"sort"
	"strings"
	"sync"
	"unicode"
)

// MatchConfigRegistry maps entity type names to their match configurations.
// It is used by the pipeline to determine allowed relation types per entity.
type MatchConfigRegistry struct {
	mu      sync.RWMutex
	configs map[string]EntityMatchConfig
}

// NewMatchConfigRegistry creates an empty registry.
func NewMatchConfigRegistry() *MatchConfigRegistry {
	return &MatchConfigRegistry{configs: make(map[string]EntityMatchConfig)}
}

// Register adds or replaces a configuration for the given entity type.
func (r *MatchConfigRegistry) Register(cfg EntityMatchConfig) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.configs[cfg.EntityType] = cfg
}

// Get returns the configuration for the given entity type, if registered.
func (r *MatchConfigRegistry) Get(entityType string) (EntityMatchConfig, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	cfg, ok := r.configs[entityType]
	return cfg, ok
}

// All returns a copy of all registered configurations.
func (r *MatchConfigRegistry) All() map[string]EntityMatchConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make(map[string]EntityMatchConfig, len(r.configs))
	for k, v := range r.configs {
		out[k] = v
	}
	return out
}

// EntityMatchConfig bundles proto-annotation-derived config for one entity type.
type EntityMatchConfig struct {
	EntityType       string           `json:"entity_type"`
	Anchors          AnchorConfig     `json:"anchors"`
	FieldWeights     []FieldWeight    `json:"field_weights"`
	Thresholds       MatchThresholds  `json:"thresholds"`
	EmbedFields      []string         `json:"embed_fields"`
	TokenFields      []string         `json:"token_fields"`
	AllowedRelations []string         `json:"allowed_relations,omitempty"`
}

// AnchorConfig defines which fields serve as identity anchors for an entity type.
type AnchorConfig struct {
	SingleAnchors    []AnchorField   `json:"single_anchors"`
	CompositeAnchors [][]AnchorField `json:"composite_anchors,omitempty"`
}

// AnchorField identifies a proto field that acts as an anchor.
type AnchorField struct {
	ProtoFieldName string              `json:"proto_field_name"`
	Normalizer     func(string) string `json:"-"`
}

// MatchThresholds controls the scoring boundaries for match decisions.
type MatchThresholds struct {
	AutoMatch  float64 `json:"auto_match"`
	ReviewZone float64 `json:"review_zone"`
}

// DefaultMatchThresholds returns sensible defaults.
func DefaultMatchThresholds() MatchThresholds {
	return MatchThresholds{
		AutoMatch:  0.85,
		ReviewZone: 0.60,
	}
}

// SimilarityFunc identifies a string similarity algorithm.
type SimilarityFunc string

const (
	SimilarityExact        SimilarityFunc = "exact"
	SimilarityJaroWinkler  SimilarityFunc = "jaro_winkler"
	SimilarityLevenshtein  SimilarityFunc = "levenshtein"
	SimilarityTokenJaccard SimilarityFunc = "token_jaccard"
)

// FieldWeight assigns a similarity function and weight to a field for scoring.
type FieldWeight struct {
	ProtoFieldName string         `json:"proto_field_name"`
	Weight         float64        `json:"weight"`
	Similarity     SimilarityFunc `json:"similarity"`
}

// NormalizeLowercaseTrim lowercases and trims whitespace.
func NormalizeLowercaseTrim(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// NormalizePhone strips non-digit characters except a leading '+'.
func NormalizePhone(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	var b strings.Builder
	for i, r := range s {
		if r == '+' && i == 0 {
			b.WriteRune(r)
		} else if unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// CollectAllowedRelations computes the union of allowed relation types from
// all registered entity configs. Returns nil if no configs define allowed
// relations (meaning: use all types).
func CollectAllowedRelations(mcr *MatchConfigRegistry) []string {
	if mcr == nil {
		return nil
	}
	seen := make(map[string]struct{})
	for _, cfg := range mcr.All() {
		for _, r := range cfg.AllowedRelations {
			seen[r] = struct{}{}
		}
	}
	if len(seen) == 0 {
		return nil
	}
	out := make([]string, 0, len(seen))
	for r := range seen {
		out = append(out, r)
	}
	sort.Strings(out)
	return out
}
