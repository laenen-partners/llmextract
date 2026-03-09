package tools

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"strings"
)

// CalculateInput is the input for the calculate tool.
type CalculateInput struct {
	Expression string `json:"expression"`
}

// CalculateOutput is the output for the calculate tool.
type CalculateOutput struct {
	Result float64 `json:"result"`
}

// Calculate evaluates a simple arithmetic expression.
// Supports +, -, *, /, parentheses, and decimal numbers.
func Calculate(expression string) (CalculateOutput, error) {
	s := strings.TrimSpace(expression)
	if s == "" {
		return CalculateOutput{}, fmt.Errorf("empty expression")
	}

	// Replace comma decimals with periods for the parser
	// But only when comma appears between digits with 1-2 trailing digits (decimal comma)
	// This is a simple heuristic; for complex cases the LLM should use parse_decimal first
	s = strings.ReplaceAll(s, " ", "")

	expr, err := parser.ParseExpr(s)
	if err != nil {
		return CalculateOutput{}, fmt.Errorf("parsing expression %q: %w", expression, err)
	}

	result, err := eval(expr)
	if err != nil {
		return CalculateOutput{}, err
	}

	return CalculateOutput{Result: result}, nil
}

func eval(expr ast.Expr) (float64, error) {
	switch e := expr.(type) {
	case *ast.BasicLit:
		if e.Kind != token.INT && e.Kind != token.FLOAT {
			return 0, fmt.Errorf("unsupported literal: %s", e.Value)
		}
		return strconv.ParseFloat(e.Value, 64)

	case *ast.UnaryExpr:
		val, err := eval(e.X)
		if err != nil {
			return 0, err
		}
		switch e.Op {
		case token.SUB:
			return -val, nil
		case token.ADD:
			return val, nil
		default:
			return 0, fmt.Errorf("unsupported unary operator: %s", e.Op)
		}

	case *ast.BinaryExpr:
		left, err := eval(e.X)
		if err != nil {
			return 0, err
		}
		right, err := eval(e.Y)
		if err != nil {
			return 0, err
		}
		switch e.Op {
		case token.ADD:
			return left + right, nil
		case token.SUB:
			return left - right, nil
		case token.MUL:
			return left * right, nil
		case token.QUO:
			if right == 0 {
				return 0, fmt.Errorf("division by zero")
			}
			return left / right, nil
		default:
			return 0, fmt.Errorf("unsupported operator: %s", e.Op)
		}

	case *ast.ParenExpr:
		return eval(e.X)

	default:
		return 0, fmt.Errorf("unsupported expression type: %T", expr)
	}
}
