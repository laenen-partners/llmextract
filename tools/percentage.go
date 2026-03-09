package tools

import (
	"fmt"
	"strconv"
	"strings"
)

// PercentageInput is the input for the parse_percentage tool.
type PercentageInput struct {
	Text string `json:"text"`
}

// PercentageOutput is the output for the parse_percentage tool.
type PercentageOutput struct {
	Decimal    float64 `json:"decimal"`
	Percentage float64 `json:"percentage"`
}

// ParsePercentage parses a percentage string into both decimal and percentage forms.
// "21%" -> {decimal: 0.21, percentage: 21.0}
func ParsePercentage(text string) (PercentageOutput, error) {
	s := strings.TrimSpace(text)
	if s == "" {
		return PercentageOutput{}, fmt.Errorf("empty input")
	}

	// Strip trailing non-numeric characters (e.g. "21%!" -> "21")
	hasPercent := strings.Contains(s, "%")
	s = strings.TrimRight(s, "%!@#$^&*()_ \t")
	s = strings.TrimSpace(s)

	// Use ParseDecimal for locale-aware parsing
	dec, err := ParseDecimal(s)
	if err != nil {
		// Fallback to plain float
		val, err2 := strconv.ParseFloat(s, 64)
		if err2 != nil {
			return PercentageOutput{}, fmt.Errorf("parsing percentage from %q: %w", text, err)
		}
		dec.Value = val
	}

	if hasPercent {
		return PercentageOutput{
			Decimal:    dec.Value / 100.0,
			Percentage: dec.Value,
		}, nil
	}

	// No % sign — if value > 1, assume it's a percentage; if <= 1, assume decimal
	if dec.Value > 1 || dec.Value < -1 {
		return PercentageOutput{
			Decimal:    dec.Value / 100.0,
			Percentage: dec.Value,
		}, nil
	}

	return PercentageOutput{
		Decimal:    dec.Value,
		Percentage: dec.Value * 100.0,
	}, nil
}
