# llmextract

Go library for LLM-powered entity extraction from markdown documents.

Takes markdown text and proto-defined entity types as input, runs a 5-step extraction pipeline via [Firebase Genkit](https://github.com/firebase/genkit), and returns structured entities with relations and confidence scores.

## Getting started

### Prerequisites

- [Go 1.25+](https://go.dev/dl/)
- [mise](https://mise.jdx.dev) (recommended) or install tools manually
- An LLM provider — [LM Studio](https://lmstudio.ai) (local, plugin included), Google AI, or any OpenAI-compatible API
- [buf](https://buf.build) for proto code generation

### Install tools

```sh
mise install
```

### Define your entity types

Create proto messages for the entities you want to extract. The pipeline derives JSON schemas from the proto descriptors and uses them to instruct the LLM on structured output.

```protobuf
// proto/entities/v1/person.proto
syntax = "proto3";
package entities.v1;
option go_package = "yourmodule/gen/entities/v1;entitiesv1";

import "google/protobuf/timestamp.proto";

message Person {
  string full_name = 1;
  string email = 2;
  string phone = 3;
  string role = 4;
}
```

```protobuf
// proto/entities/v1/invoice.proto
syntax = "proto3";
package entities.v1;
option go_package = "yourmodule/gen/entities/v1;entitiesv1";

import "google/protobuf/timestamp.proto";

message Invoice {
  string invoice_number = 1;
  google.protobuf.Timestamp date = 2;
  google.protobuf.Timestamp due_date = 3;
  string currency = 4;
  double total_amount = 5;
  double tax_amount = 6;
  repeated LineItem line_items = 7;
  string seller_name = 8;
  string buyer_name = 9;
}

message LineItem {
  string description = 1;
  double quantity = 2;
  double unit_price = 3;
  double amount = 4;
}
```

Generate Go code:

```sh
buf generate
```

### Extract entities

```go
package main

import (
    "context"
    "encoding/json"
    "fmt"

    "github.com/firebase/genkit/go/genkit"

    "github.com/laenen-partners/llmextract/pipeline"
    "github.com/laenen-partners/llmextract/registry"
    "github.com/laenen-partners/llmextract/tools"

    entitiesv1 "yourmodule/gen/entities/v1"
)

func main() {
    ctx := context.Background()

    // Initialize Genkit with your LLM provider (see "LM Studio" section below).
    g := genkit.Init(ctx, genkit.WithPlugins(/* your provider plugin */))

    // Register entity types — the library derives JSON schemas from proto descriptors.
    reg := registry.New()
    reg.Register(&entitiesv1.Person{})
    reg.Register(&entitiesv1.Invoice{})

    // Register parsing tools (money, date, decimal, percentage, calculate).
    parsingTools := tools.RegisterAll(g)

    // Create and run the pipeline.
    p := pipeline.New(g, reg,
        pipeline.WithModel("lmstudio/google/gemma-3-4b"),
        pipeline.WithTools(parsingTools),
    )

    result, err := p.Extract(ctx, markdownDocument)
    if err != nil {
        panic(err)
    }

    out, _ := json.MarshalIndent(result, "", "  ")
    fmt.Println(string(out))
}
```

## Examples

Working examples are included in `examples/`. Each is a self-contained Go module with its own proto definitions, generated code, and sample documents.

### basic-lmstudio

Extracts Person, Organisation, Address, and Invoice entities from markdown documents using [LM Studio](https://lmstudio.ai) as a local LLM provider.

**What it demonstrates:**
- Defining entity types as proto messages
- Initializing Genkit with a local LLM (OpenAI-compatible API)
- Registering entity types and parsing tools
- Running the full extraction pipeline
- Processing multi-document input (cover letters, invoices, payment reminders)

**Sample documents included:**
- `testdata/sample_invoice.md` — Cover letter + invoice from Acme Corporation to John Doe (single invoice, 3 line items)
- `testdata/complex_correspondence.md` — Multi-document correspondence: cover letter + 2 invoices + payment reminder between Benelux Logistics Group and NordTech Solutions

**Run it:**

```sh
# Using Taskfile (from repo root)
task example:simple                              # sample invoice
task example:complex                             # complex correspondence
task example -- testdata/sample_invoice.md       # custom file
task example -- -i testdata/sample_invoice.md -m qwen2.5-7b-instruct  # custom model

# Or directly
cd examples/basic-lmstudio
go run . -i testdata/sample_invoice.md
go run . -i testdata/complex_correspondence.md
```

**Flags:**

| Flag | Default | Description |
|------|---------|-------------|
| `-i` | (required) | Path to input markdown file |
| `-m` | `google/gemma-3-4b` | Model name loaded in LM Studio |
| `-url` | `http://localhost:1234/v1` | LM Studio server URL |

### Writing your own example

1. Create a new directory under `examples/`
2. Add a `go.mod` with a `replace` directive pointing to the library root:
   ```
   replace github.com/laenen-partners/llmextract => ../..
   ```
3. Define your entity types as proto messages in `proto/`
4. Add `buf.yaml` and `buf.gen.yaml`, then run `buf generate`
5. Write a `main.go` that initializes Genkit, registers types, and runs the pipeline
6. Add the module to `Taskfile.yml` under the `tidy` task

## Proto design guidelines

The quality of extraction depends heavily on how you define your proto messages. The LLM sees the JSON schema derived from your proto — clear structure leads to better results.

### Field naming

Use descriptive, unambiguous field names. The LLM uses field names as hints for what to extract.

```protobuf
// Good — clear what each field represents
string full_name = 1;
string invoice_number = 2;
double total_amount = 3;

// Avoid — ambiguous
string name = 1;      // name of what?
string number = 2;    // what kind of number?
double amount = 3;    // which amount?
```

### Use appropriate types

- `string` for text, identifiers, codes
- `double` for monetary amounts and quantities
- `int32`/`int64` for whole numbers
- `google.protobuf.Timestamp` for dates and times — the pipeline normalises date strings to RFC 3339
- `repeated` for lists (line items, tags, etc.)
- Nested messages for structured sub-objects (line items, tax breakdowns)

### Keep entities focused

Each message should represent one real-world entity type. Avoid combining unrelated concepts.

```protobuf
// Good — separate entity types with clear boundaries
message Person { ... }
message Organisation { ... }
message Invoice { ... }

// Avoid — kitchen-sink messages
message DocumentData {
  string person_name = 1;
  string company_name = 2;
  string invoice_number = 3;
  // ...
}
```

### Separate seller/buyer with relations

Don't embed references to other entity types as sub-messages. Instead, use flat string fields for names/IDs and let the pipeline discover relations between entities automatically.

```protobuf
// Good — flat references, pipeline discovers relations
message Invoice {
  string seller_name = 1;
  string buyer_name = 2;
  string seller_tax_id = 3;
}

// Avoid — nested entity references
message Invoice {
  Organisation seller = 1;
  Person buyer = 2;
}
```

### Register only what you need

Only register entity types that are relevant to your documents. Registering unnecessary types increases LLM token usage and can produce false positives.

```go
// Processing invoices? Register invoice-related types.
reg.Register(&entitiesv1.Person{})
reg.Register(&entitiesv1.Organisation{})
reg.Register(&entitiesv1.Address{})
reg.Register(&entitiesv1.Invoice{})

// Don't register types you don't expect in the input.
```

## Pipeline steps

1. **Discovery** — LLM identifies which registered entity types are present
2. **Extraction** — LLM extracts all instances of each type using JSON schemas
3. **Validation & Correction** — Three-layer validation (protovalidate, semantic, plugins) with LLM correction loop
4. **Relation Resolution** — LLM identifies same_as pairs, semantic relations, and entity roles
5. **Inference** — LLM discovers implied entities and relations via coreference resolution

## Configuration

```go
p := pipeline.New(g, reg,
    pipeline.WithModel("lmstudio/google/gemma-3-4b"),    // LLM model
    pipeline.WithTools(parsingTools),                      // deterministic tools
    pipeline.WithMaxCorrectionRounds(3),                   // validation rounds
    pipeline.WithMinConfidence(0.6),                       // confidence threshold
    pipeline.WithStepTimeout(2 * time.Minute),             // per-step timeout
    pipeline.WithRunner(myRunner),                         // custom step runner
    pipeline.WithValidators(vr),                           // semantic validators
    pipeline.WithPlugins(pr),                              // validation plugins
    pipeline.WithMatchConfigs(mcr),                        // relation type config
)
```

## Durable execution

The `runners/dbos/` submodule provides a step runner backed by [DBOS](https://docs.dbos.dev/) durable steps. Each pipeline step is checkpointed to Postgres — if the process crashes, completed steps replay from storage without re-executing LLM calls.

```go
import dbosrunner "github.com/laenen-partners/llmextract/runners/dbos"

// Inside a DBOS workflow:
r := dbosrunner.New(dbosCtx)
p := pipeline.New(g, reg, pipeline.WithRunner(r))
result, err := p.Extract(ctx, document)
```

This is a separate Go module to keep the core library free of DBOS dependencies.

## LM Studio plugin

The `plugins/lmstudio/` submodule provides a ready-to-use [Genkit](https://github.com/firebase/genkit) plugin for [LM Studio](https://lmstudio.ai). LM Studio runs LLMs locally and exposes an OpenAI-compatible API — this plugin registers those models with Genkit.

```sh
go get github.com/laenen-partners/llmextract/plugins/lmstudio
```

```go
import (
    "github.com/firebase/genkit/go/genkit"
    "github.com/laenen-partners/llmextract/plugins/lmstudio"
    "github.com/laenen-partners/llmextract/pipeline"
)

// Initialize Genkit with the LM Studio plugin.
g := genkit.Init(ctx, genkit.WithPlugins(&lmstudio.LMStudio{
    Models: []lmstudio.ModelDef{{Name: "google/gemma-3-4b"}},
}))

// Use the model in the pipeline (prefixed with "lmstudio/").
p := pipeline.New(g, reg,
    pipeline.WithModel("lmstudio/google/gemma-3-4b"),
    pipeline.WithTools(parsingTools),
)
result, err := p.Extract(ctx, document)
```

By default it connects to `http://localhost:1234/v1`. Override with `BaseURL`:

```go
&lmstudio.LMStudio{
    BaseURL: "http://192.168.1.100:1234/v1",
    Models:  []lmstudio.ModelDef{{Name: "google/gemma-3-4b"}},
}
```

This is a separate Go module so the core library stays free of the OpenAI SDK dependency. If you use a different LLM provider (Google AI, Anthropic, etc.), you don't need this module — just pass the appropriate Genkit plugin.

## Project structure

```
extract.go              Core types (ExtractionOutput, ExtractedEntity, etc.)
match_config.go         MatchConfigRegistry for relation type configuration
registry/               Entity type registry (proto message -> JSON Schema)
schema/                 Proto MessageDescriptor -> JSON Schema conversion
pipeline/               5-step extraction pipeline
validation/             Validation framework (ValidatorRegistry, PluginRegistry)
tools/                  Deterministic parsing tools (money, date, decimal, etc.)
runner/                 StepRunner interface + direct (synchronous) runner
plugins/lmstudio/       Genkit plugin for LM Studio (separate Go module)
runners/dbos/           DBOS durable step runner (separate Go module)
examples/               Working examples
```

## Testing

```sh
task test           # run all tests
task test:cover     # run with coverage
```
