package pipeline

import (
	llmextract "github.com/laenen-partners/llmextract"
)

const (
	validationErrorPenalty   = 0.15
	correctionSuccessRestore = 0.05
)

func (p *Pipeline) scoreConfidence(corrections []llmextract.CorrectionEntry, entities []llmextract.ExtractedEntity) ([]llmextract.ExtractedEntity, error) {
	// Count corrections per entity
	correctionCount := make(map[string]int)
	for _, c := range corrections {
		correctionCount[c.EntityID]++
	}

	scored := make([]llmextract.ExtractedEntity, len(entities))
	copy(scored, entities)

	for i := range scored {
		e := &scored[i]
		numCorrections := correctionCount[e.EntityID]

		if numCorrections > 0 {
			// Penalize for needing corrections
			penalty := float64(numCorrections) * validationErrorPenalty
			// Partially restore for successful corrections
			restore := float64(numCorrections) * correctionSuccessRestore
			e.Confidence = e.Confidence - penalty + restore
		}

		// Clamp to [0, 1]
		if e.Confidence < 0 {
			e.Confidence = 0
		}
		if e.Confidence > 1 {
			e.Confidence = 1
		}
	}

	return scored, nil
}
