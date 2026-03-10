// Example program that extracts Person, Organisation, Address, and Invoice
// entities from markdown documents.
//
// The program auto-detects the LLM provider based on environment variables:
//   - GEMINI_API_KEY or GOOGLE_API_KEY → Google Gemini API
//   - Otherwise → LM Studio (local)
//
// Usage:
//
//	go run . -i testdata/sample_invoice.md
//	go run . -i testdata/complex_correspondence.md
//	go run . -i testdata/sample_invoice.md -m gemini-2.5-flash
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/firebase/genkit/go/genkit"
	"github.com/firebase/genkit/go/plugins/googlegenai"

	"github.com/laenen-partners/llmextract/plugins/lmstudio"

	entitiesv1 "github.com/laenen-partners/llmextract/examples/basic-lmstudio/gen/entities/v1"
	"github.com/laenen-partners/llmextract/pipeline"
	"github.com/laenen-partners/llmextract/registry"
	"github.com/laenen-partners/llmextract/tools"
)

func loadDotEnv() {
	f, err := os.Open(".env")
	if err != nil {
		return
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if k, v, ok := strings.Cut(line, "="); ok {
			// Strip inline comments (e.g. "13s # comment").
			if i := strings.Index(v, " #"); i >= 0 {
				v = strings.TrimSpace(v[:i])
			}
			if os.Getenv(k) == "" {
				os.Setenv(k, v)
			}
		}
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// geminiAPIKey returns the Gemini API key if set.
func geminiAPIKey() string {
	if k := os.Getenv("GEMINI_API_KEY"); k != "" {
		return k
	}
	return os.Getenv("GOOGLE_API_KEY")
}

func defaultModel() string {
	if geminiAPIKey() != "" {
		return "gemini-2.5-flash"
	}
	return envOr("LMSTUDIO_MODEL", "qwen2.5-32b-instruct")
}

func main() {
	loadDotEnv()
	inputFile := flag.String("i", "", "Path to input markdown file (required)")
	model := flag.String("m", defaultModel(), "Model name")
	lmStudioURL := flag.String("url", envOr("LMSTUDIO_URL", lmstudio.DefaultURL), "LM Studio server URL (ignored when using Gemini)")
	flag.Parse()

	if *inputFile == "" {
		fmt.Fprintln(os.Stderr, "error: -i (input file) is required")
		flag.Usage()
		os.Exit(1)
	}

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))

	if err := run(context.Background(), *inputFile, *model, *lmStudioURL); err != nil {
		slog.Error("extraction failed", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, inputFile, model, lmStudioURL string) error {
	// Read the input document.
	doc, err := os.ReadFile(inputFile)
	if err != nil {
		return fmt.Errorf("reading input: %w", err)
	}

	// Auto-detect provider based on environment variables.
	var modelRef string
	var genkitOpts []genkit.GenkitOption

	if apiKey := geminiAPIKey(); apiKey != "" {
		// Google Gemini API
		if !strings.HasPrefix(model, "googleai/") {
			modelRef = "googleai/" + model
		} else {
			modelRef = model
		}
		genkitOpts = append(genkitOpts, genkit.WithPlugins(&googlegenai.GoogleAI{APIKey: apiKey}))
		slog.Info("initializing with Google Gemini", "model", modelRef)
	} else {
		// LM Studio fallback
		if !strings.HasPrefix(model, "lmstudio/") {
			modelRef = "lmstudio/" + model
		} else {
			modelRef = model
		}
		genkitOpts = append(genkitOpts, genkit.WithPlugins(&lmstudio.LMStudio{
			BaseURL: lmStudioURL,
			Models:  []lmstudio.ModelDef{{Name: model}},
		}))
		slog.Info("initializing with LM Studio", "model", modelRef, "url", lmStudioURL)
	}

	g := genkit.Init(ctx, genkitOpts...)

	// Register entity types. The registry derives JSON schemas from the
	// proto message descriptors — the LLM uses these schemas to produce
	// structured output.
	reg := registry.New()
	reg.Register(&entitiesv1.Person{})
	reg.Register(&entitiesv1.Organisation{})
	reg.Register(&entitiesv1.Address{})
	reg.Register(&entitiesv1.Invoice{})

	// Register deterministic parsing tools (money, date, decimal, etc.).
	// The LLM calls these tools instead of guessing numeric/date formats.
	parsingTools := tools.RegisterAll(g)

	// Create the extraction pipeline.
	pipelineOpts := []pipeline.Option{
		pipeline.WithModel(modelRef),
		pipeline.WithTools(parsingTools),
	}
	// Rate limit delay between LLM calls (e.g. "13s" for Gemini free tier).
	if d := os.Getenv("REQUEST_DELAY"); d != "" {
		if dur, err := time.ParseDuration(d); err == nil {
			pipelineOpts = append(pipelineOpts, pipeline.WithRequestDelay(dur))
		}
	}
	p := pipeline.New(g, reg, pipelineOpts...)

	// Run extraction.
	slog.Info("extracting entities", "file", inputFile)
	result, err := p.Extract(ctx, string(doc))
	if err != nil {
		return fmt.Errorf("extraction: %w", err)
	}

	slog.Info("extraction complete",
		"entities", len(result.Entities),
		"relations", len(result.Relations),
		"implied_entities", len(result.ImpliedEntities),
		"confidence", fmt.Sprintf("%.2f", result.OverallConfidence),
		"duration_ms", result.ProcessingMeta.DurationMs,
	)

	// Print JSON output to stdout.
	output, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal output: %w", err)
	}
	fmt.Println(string(output))
	return nil
}
