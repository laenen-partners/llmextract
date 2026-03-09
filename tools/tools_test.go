package tools

import (
	"math"
	"testing"
)

func approxEqual(a, b float64) bool {
	return math.Abs(a-b) < 0.001
}

// --- ParseDecimal ---

func TestParseDecimal(t *testing.T) {
	tests := []struct {
		input string
		want  float64
	}{
		{"1234", 1234},
		{"1234.56", 1234.56},
		{"1,234.56", 1234.56},
		{"1.234,56", 1234.56},
		{"1 234,56", 1234.56},
		{"-1234.56", -1234.56},
		{"+1234.56", 1234.56},
		{"0.5", 0.5},
		{"0,5", 0.5},
		{"1.234.567", 1234567},
		{"1,234,567", 1234567},
		{"1.234.567,89", 1234567.89},
		{"1,234,567.89", 1234567.89},
		{" 42 ", 42},
		{"100", 100},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseDecimal(tt.input)
			if err != nil {
				t.Fatalf("ParseDecimal(%q) error: %v", tt.input, err)
			}
			if !approxEqual(got.Value, tt.want) {
				t.Errorf("ParseDecimal(%q) = %v, want %v", tt.input, got.Value, tt.want)
			}
		})
	}
}

func TestParseDecimalErrors(t *testing.T) {
	_, err := ParseDecimal("")
	if err == nil {
		t.Error("expected error for empty input")
	}

	_, err = ParseDecimal("abc")
	if err == nil {
		t.Error("expected error for non-numeric input")
	}
}

// --- ParseMoney ---

func TestParseMoney(t *testing.T) {
	tests := []struct {
		input    string
		amount   float64
		currency string
	}{
		{"EUR 4.350,00", 4350.00, "EUR"},
		{"$1,234.56", 1234.56, "USD"},
		{"€1.234,56", 1234.56, "EUR"},
		{"£500", 500, "GBP"},
		{"4,350.00 EUR", 4350.00, "EUR"},
		{"USD 100", 100, "USD"},
		{"¥10000", 10000, "JPY"},
		{"1 234,56 SEK", 1234.56, "SEK"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseMoney(tt.input)
			if err != nil {
				t.Fatalf("ParseMoney(%q) error: %v", tt.input, err)
			}
			if !approxEqual(got.Amount, tt.amount) {
				t.Errorf("ParseMoney(%q).Amount = %v, want %v", tt.input, got.Amount, tt.amount)
			}
			if got.Currency != tt.currency {
				t.Errorf("ParseMoney(%q).Currency = %q, want %q", tt.input, got.Currency, tt.currency)
			}
		})
	}
}

// --- ParseDate ---

func TestParseDate(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"2025-01-15", "2025-01-15T00:00:00Z"},
		{"2025-01-15T10:30:00Z", "2025-01-15T10:30:00Z"},
		{"January 15, 2025", "2025-01-15T00:00:00Z"},
		{"15 January 2025", "2025-01-15T00:00:00Z"},
		{"Jan 15, 2025", "2025-01-15T00:00:00Z"},
		{"15 Jan 2025", "2025-01-15T00:00:00Z"},
		{"15.01.2025", "2025-01-15T00:00:00Z"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseDate(tt.input)
			if err != nil {
				t.Fatalf("ParseDate(%q) error: %v", tt.input, err)
			}
			if got.ISO != tt.want {
				t.Errorf("ParseDate(%q) = %q, want %q", tt.input, got.ISO, tt.want)
			}
		})
	}
}

func TestParseDateErrors(t *testing.T) {
	_, err := ParseDate("")
	if err == nil {
		t.Error("expected error for empty input")
	}

	_, err = ParseDate("not a date")
	if err == nil {
		t.Error("expected error for invalid date")
	}
}

// --- ParsePercentage ---

func TestParsePercentage(t *testing.T) {
	tests := []struct {
		input      string
		decimal    float64
		percentage float64
	}{
		{"21%", 0.21, 21.0},
		{"5.5%", 0.055, 5.5},
		{"100%", 1.0, 100.0},
		{"0%", 0, 0},
		{"0.21", 0.21, 21.0},
		{"0.5", 0.5, 50.0},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParsePercentage(tt.input)
			if err != nil {
				t.Fatalf("ParsePercentage(%q) error: %v", tt.input, err)
			}
			if !approxEqual(got.Decimal, tt.decimal) {
				t.Errorf("ParsePercentage(%q).Decimal = %v, want %v", tt.input, got.Decimal, tt.decimal)
			}
			if !approxEqual(got.Percentage, tt.percentage) {
				t.Errorf("ParsePercentage(%q).Percentage = %v, want %v", tt.input, got.Percentage, tt.percentage)
			}
		})
	}
}

// --- Calculate ---

func TestCalculate(t *testing.T) {
	tests := []struct {
		input string
		want  float64
	}{
		{"1 + 2", 3},
		{"20 * 125.00", 2500},
		{"20 * 125.00 + 8 * 150.00 + 650.00", 4350},
		{"(10 + 5) * 2", 30},
		{"100 / 4", 25},
		{"10 - 3", 7},
		{"-5 + 10", 5},
		{"(1 + 2) * (3 + 4)", 21},
		{"2.5 * 4.0", 10},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := Calculate(tt.input)
			if err != nil {
				t.Fatalf("Calculate(%q) error: %v", tt.input, err)
			}
			if !approxEqual(got.Result, tt.want) {
				t.Errorf("Calculate(%q) = %v, want %v", tt.input, got.Result, tt.want)
			}
		})
	}
}

func TestCalculateErrors(t *testing.T) {
	_, err := Calculate("")
	if err == nil {
		t.Error("expected error for empty expression")
	}

	_, err = Calculate("10 / 0")
	if err == nil {
		t.Error("expected error for division by zero")
	}
}
