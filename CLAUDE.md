# llmextract

Go library for LLM-powered entity extraction from markdown documents.

See `~/.claude/CLAUDE.md` for org-wide Go service standards (tooling, conventions).

## Overview

This is a **library module** (not a server). It provides a pipeline that:
1. Takes markdown text + registered proto entity definitions as input
2. Uses an LLM (via Firebase Genkit) to extract structured entities
3. Returns validated, typed entities with relations and confidence scores

## Project structure

```
extract.go                Core types (ExtractionOutput, ExtractedEntity, etc.)
match_config.go           MatchConfigRegistry for relation type configuration
registry/                 Entity type registry (proto -> JSON Schema)
schema/                   Proto MessageDescriptor -> JSON Schema conversion
pipeline/                 5-step extraction pipeline (discovery, extraction, correction, relations, inference)
validation/               Validation framework (ValidationResult, ValidatorRegistry, PluginRegistry)
tools/                    Deterministic parsing tools for Genkit (money, date, decimal, percentage, calculate)
runner/                   Step runner interface + direct (synchronous) implementation
plugins/lmstudio/         Genkit plugin for LM Studio (separate Go module)
runners/dbos/             DBOS durable step runner (separate Go module)
examples/basic-lmstudio/  End-to-end example using LM Studio (separate Go module)
```

## Pipeline steps

1. **Discovery** — LLM identifies which registered entity types are present in the document
2. **Extraction** — LLM extracts all instances of each discovered type using JSON schemas
3. **Validation & Correction** — Three-layer validation (protovalidate, semantic, plugins) with LLM correction loop
4. **Relation Resolution** — LLM identifies same_as pairs, semantic relations, and entity roles
5. **Inference** — LLM discovers implied entities and relations via coreference resolution

## Usage

```go
import (
    "github.com/laenen-partners/llmextract/pipeline"
    "github.com/laenen-partners/llmextract/registry"
    "github.com/laenen-partners/llmextract/tools"
)

// 1. Create registry and register entity types (proto messages)
reg := registry.New()
reg.Register(&entitiesv1.Person{})
reg.Register(&entitiesv1.Invoice{})

// 2. Initialize Genkit
g, _ := genkit.Init(ctx, &genkit.Options{})

// 3. Register tools and create pipeline
t := tools.RegisterAll(g)
p := pipeline.New(g, reg, pipeline.WithTools(t))

// 4. Extract
result, _ := p.Extract(ctx, markdownDocument)
```

## Durable execution (DBOS)

The `runners/dbos/` submodule provides a `StepRunner` backed by DBOS durable steps.
Each pipeline step is checkpointed to Postgres — if the process crashes, completed
steps are replayed from storage without re-executing LLM calls.

```go
import dbosrunner "github.com/laenen-partners/llmextract/runners/dbos"

// In a DBOS workflow function:
r := dbosrunner.New(dbosCtx)
p := pipeline.New(g, reg, pipeline.WithRunner(r))
result, err := p.Extract(ctx, document)
```

This is a separate Go module to keep the core library free of DBOS dependencies.
Services that don't use DBOS use `direct.New()` (the default).

## Dependencies

- **firebase/genkit** — LLM framework (models, tools, structured output)
- **protovalidate** — Proto field constraint validation
- **protobuf** — Proto message handling, JSON marshalling
- **dbos-transact-golang** — (runners/dbos only) Durable step execution
- **openai/openai-go** — (plugins/lmstudio only) OpenAI-compatible API client

## What this module does NOT include

- CLI or server — consumers build their own
- Storage / persistence — use a separate entity store
- Entity proto definitions — consumers define their own entity types
- Matching / deduplication — handled by entity store consumers
