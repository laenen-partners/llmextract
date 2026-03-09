package tools

import (
	"fmt"
	"strconv"
	"strings"
)

// DecimalInput is the input for the parse_decimal tool.
type DecimalInput struct {
	Text string `json:"text"`
}

// DecimalOutput is the output for the parse_decimal tool.
type DecimalOutput struct {
	Value float64 `json:"value"`
}

// ParseDecimal parses a locale-aware decimal number string into a float64.
// It auto-detects European (1.234,56) vs US (1,234.56) format.
func ParseDecimal(text string) (DecimalOutput, error) {
	s := strings.TrimSpace(text)
	if s == "" {
		return DecimalOutput{}, fmt.Errorf("empty input")
	}

	// Handle negative
	negative := false
	if s[0] == '-' {
		negative = true
		s = s[1:]
	} else if s[0] == '+' {
		s = s[1:]
	}

	// Remove spaces (e.g., "1 234,56")
	s = strings.ReplaceAll(s, " ", "")
	s = strings.ReplaceAll(s, "\u00a0", "") // non-breaking space

	// Find last comma and last period
	lastComma := strings.LastIndex(s, ",")
	lastPeriod := strings.LastIndex(s, ".")

	var cleaned string
	switch {
	case lastComma == -1 && lastPeriod == -1:
		// No separators: "1234"
		cleaned = s
	case lastComma == -1:
		// Only periods: could be "1.234" (thousands) or "1.23" (decimal)
		// If there's exactly one period and 1-2 digits after it, treat as decimal
		if strings.Count(s, ".") == 1 {
			afterDot := len(s) - lastPeriod - 1
			if afterDot <= 2 {
				cleaned = s // already US format
			} else {
				// "1.234" -> thousands separator
				cleaned = strings.ReplaceAll(s, ".", "")
			}
		} else {
			// Multiple periods: "1.234.567" -> thousands separators
			cleaned = strings.ReplaceAll(s, ".", "")
		}
	case lastPeriod == -1:
		// Only commas: could be "1,234" (thousands) or "1,23" (decimal)
		if strings.Count(s, ",") == 1 {
			afterComma := len(s) - lastComma - 1
			if afterComma <= 2 {
				// European decimal: "1.234,56" -> "1234.56"
				cleaned = strings.ReplaceAll(s, ",", ".")
			} else {
				// Thousands separator: "1,234"
				cleaned = strings.ReplaceAll(s, ",", "")
			}
		} else {
			// Multiple commas: "1,234,567" -> thousands separators
			cleaned = strings.ReplaceAll(s, ",", "")
		}
	case lastComma > lastPeriod:
		// Comma is last: European format "1.234,56"
		cleaned = strings.ReplaceAll(s, ".", "")
		cleaned = strings.ReplaceAll(cleaned, ",", ".")
	default:
		// Period is last: US format "1,234.56"
		cleaned = strings.ReplaceAll(s, ",", "")
	}

	val, err := strconv.ParseFloat(cleaned, 64)
	if err != nil {
		return DecimalOutput{}, fmt.Errorf("parsing %q: %w", text, err)
	}

	if negative {
		val = -val
	}

	return DecimalOutput{Value: val}, nil
}
