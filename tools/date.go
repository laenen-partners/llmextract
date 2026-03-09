package tools

import (
	"fmt"
	"strings"
	"time"
)

// DateInput is the input for the parse_date tool.
type DateInput struct {
	Text string `json:"text"`
}

// DateOutput is the output for the parse_date tool.
type DateOutput struct {
	ISO string `json:"iso"`
}

// Formats tried in order of specificity.
var dateFormats = []string{
	// ISO
	time.RFC3339,
	"2006-01-02T15:04:05",
	"2006-01-02",

	// European dot
	"02.01.2006",

	// European slash (day/month/year)
	"02/01/2006",

	// US slash (month/day/year)
	"01/02/2006",

	// Long formats
	"January 2, 2006",
	"2 January 2006",
	"Jan 2, 2006",
	"2 Jan 2006",
	"January 02, 2006",
	"02 January 2006",

	// Dash formats
	"02-01-2006",
	"01-02-2006",
}

// ParseDate parses a date string in various formats into ISO 8601.
func ParseDate(text string) (DateOutput, error) {
	s := strings.TrimSpace(text)
	if s == "" {
		return DateOutput{}, fmt.Errorf("empty input")
	}

	for _, layout := range dateFormats {
		t, err := time.Parse(layout, s)
		if err == nil {
			return DateOutput{ISO: t.UTC().Format(time.RFC3339)}, nil
		}
	}

	return DateOutput{}, fmt.Errorf("unable to parse date %q", text)
}
