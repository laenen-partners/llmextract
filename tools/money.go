package tools

import (
	"fmt"
	"regexp"
	"strings"
)

// MoneyInput is the input for the parse_money tool.
type MoneyInput struct {
	Text string `json:"text"`
}

// MoneyOutput is the output for the parse_money tool.
type MoneyOutput struct {
	Amount   float64 `json:"amount"`
	Currency string  `json:"currency"`
}

var currencySymbols = map[string]string{
	"$": "USD",
	"€": "EUR",
	"£": "GBP",
	"¥": "JPY",
	"₹": "INR",
	"₩": "KRW",
	"₽": "RUB",
	"₣": "CHF",
	"R$": "BRL",
	"kr": "SEK", // also NOK, DKK — defaults to SEK
	"zł": "PLN",
	"Kč": "CZK",
	"Ft": "HUF",
}

var isoCodePattern = regexp.MustCompile(`(?i)\b([A-Z]{3})\b`)

// ParseMoney parses a monetary amount string into a numeric value and ISO 4217 currency code.
func ParseMoney(text string) (MoneyOutput, error) {
	s := strings.TrimSpace(text)
	if s == "" {
		return MoneyOutput{}, fmt.Errorf("empty input")
	}

	// Detect currency
	currency := detectCurrency(s)

	// Strip currency symbols and codes to get the numeric part
	numStr := stripCurrency(s)

	// Parse the numeric part using ParseDecimal
	dec, err := ParseDecimal(numStr)
	if err != nil {
		return MoneyOutput{}, fmt.Errorf("parsing amount from %q: %w", text, err)
	}

	return MoneyOutput{
		Amount:   dec.Value,
		Currency: currency,
	}, nil
}

func detectCurrency(s string) string {
	// Check for multi-char symbols first (R$, kr, etc.)
	for sym, code := range currencySymbols {
		if len(sym) > 1 && strings.Contains(s, sym) {
			return code
		}
	}

	// Check for single-char symbols
	for sym, code := range currencySymbols {
		if len(sym) == 1 && strings.Contains(s, sym) {
			return code
		}
	}

	// Check for ISO currency codes (3 uppercase letters)
	matches := isoCodePattern.FindStringSubmatch(s)
	if len(matches) > 1 {
		return strings.ToUpper(matches[1])
	}

	return ""
}

func stripCurrency(s string) string {
	// Remove known symbols (longest first)
	for sym := range currencySymbols {
		s = strings.ReplaceAll(s, sym, "")
	}

	// Remove ISO codes
	s = isoCodePattern.ReplaceAllString(s, "")

	// Remove common decorators
	s = strings.ReplaceAll(s, "-", "")
	s = strings.TrimSpace(s)

	return s
}
