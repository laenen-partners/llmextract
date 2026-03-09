package tools

import (
	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"
)

// RegisterAll registers all deterministic parsing tools with Genkit
// and returns them as ToolRefs to pass to ai.WithTools().
func RegisterAll(g *genkit.Genkit) []ai.ToolRef {
	money := genkit.DefineTool(g,
		"parse_money",
		"Parse a money/currency string into a numeric amount and ISO 4217 currency code. "+
			"Use this for any monetary value like 'EUR 4.350,00', '$1,234.56', '£500'. "+
			"Handles European and US number formats automatically.",
		func(ctx *ai.ToolContext, input MoneyInput) (MoneyOutput, error) {
			return ParseMoney(input.Text)
		},
	)

	date := genkit.DefineTool(g,
		"parse_date",
		"Parse a date string in any common format into ISO 8601 (RFC 3339). "+
			"Use this for dates like 'January 15, 2025', '15/01/2025', '2025-01-15', '15 Jan 2025'. "+
			"Returns the date in UTC format.",
		func(ctx *ai.ToolContext, input DateInput) (DateOutput, error) {
			return ParseDate(input.Text)
		},
	)

	decimal := genkit.DefineTool(g,
		"parse_decimal",
		"Parse a locale-aware decimal number string into a float64. "+
			"Use this for numbers with ambiguous formatting like '1.234,56' (European) or '1,234.56' (US). "+
			"Automatically detects the format.",
		func(ctx *ai.ToolContext, input DecimalInput) (DecimalOutput, error) {
			return ParseDecimal(input.Text)
		},
	)

	percentage := genkit.DefineTool(g,
		"parse_percentage",
		"Parse a percentage string into both decimal and percentage forms. "+
			"Use this for values like '21%', '5.5%'. Returns both decimal (0.21) and percentage (21.0) forms.",
		func(ctx *ai.ToolContext, input PercentageInput) (PercentageOutput, error) {
			return ParsePercentage(input.Text)
		},
	)

	calc := genkit.DefineTool(g,
		"calculate",
		"Evaluate an arithmetic expression and return the result. "+
			"Use this for any calculation like '20 * 125.00 + 8 * 150.00 + 650.00'. "+
			"Supports +, -, *, /, and parentheses.",
		func(ctx *ai.ToolContext, input CalculateInput) (CalculateOutput, error) {
			return Calculate(input.Expression)
		},
	)

	return []ai.ToolRef{money, date, decimal, percentage, calc}
}
