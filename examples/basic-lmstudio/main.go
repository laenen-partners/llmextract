// Example program that extracts Person, Organisation, Address, and Invoice
// entities from markdown documents using LM Studio as the LLM provider.
//
// Prerequisites:
//
//	1. Install and start LM Studio (https://lmstudio.ai)
//	2. Load a model (e.g. gemma-3-4b or qwen2.5-7b)
//	3. Start the local server (default: http://localhost:1234)
//
// Usage:
//
//	go run . -i testdata/sample_invoice.md
//	go run . -i testdata/complex_correspondence.md
//	go run . -i testdata/sample_invoice.md -m qwen2.5-7b-instruct
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

	"github.com/firebase/genkit/go/genkit"

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

func main() {
	loadDotEnv()
	inputFile := flag.String("i", "", "Path to input markdown file (required)")
	model := flag.String("m", envOr("LMSTUDIO_MODEL", "google/gemma-3-4b"), "Model name in LM Studio")
	lmStudioURL := flag.String("url", envOr("LMSTUDIO_URL", lmstudio.DefaultURL), "LM Studio server URL")
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

	// Build fully qualified model reference for Genkit.
	modelRef := model
	if !strings.HasPrefix(modelRef, "lmstudio/") {
		modelRef = "lmstudio/" + model
	}

	slog.Info("initializing", "model", modelRef, "url", lmStudioURL)

	// Initialize Genkit with LM Studio plugin.
	g := genkit.Init(ctx, genkit.WithPlugins(&lmstudio.LMStudio{
		BaseURL: lmStudioURL,
		Models:  []lmstudio.ModelDef{{Name: model}},
	}))

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
	p := pipeline.New(g, reg,
		pipeline.WithModel(modelRef),
		pipeline.WithTools(parsingTools),
	)

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
