// Package lmstudio provides a Genkit plugin for LM Studio's OpenAI-compatible API.
//
// LM Studio (https://lmstudio.ai) runs LLMs locally and exposes an
// OpenAI-compatible HTTP API. This plugin registers models loaded in LM Studio
// with Genkit so they can be used with llmextract's extraction pipeline.
//
// # Quick start
//
//	g := genkit.Init(ctx, genkit.WithPlugins(&lmstudio.LMStudio{
//	    Models: []lmstudio.ModelDef{{Name: "google/gemma-3-4b"}},
//	}))
//
//	p := pipeline.New(g, reg, pipeline.WithModel("lmstudio/google/gemma-3-4b"))
//
// # Custom URL
//
// By default the plugin connects to http://localhost:1234/v1. Override with BaseURL:
//
//	&lmstudio.LMStudio{
//	    BaseURL: "http://192.168.1.100:1234/v1",
//	    Models:  []lmstudio.ModelDef{{Name: "google/gemma-3-4b"}},
//	}
//
// # Model capabilities
//
// By default models are registered with basic text capabilities. To declare
// support for structured output, tool use, or other features, set Supports:
//
//	lmstudio.ModelDef{
//	    Name: "google/gemma-3-4b",
//	    Supports: &ai.ModelSupports{
//	        Tools:      true,
//	        ToolChoice: true,
//	    },
//	}
package lmstudio

import (
	"context"

	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/core/api"
	"github.com/firebase/genkit/go/plugins/compat_oai"
)

const (
	provider   = "lmstudio"
	DefaultURL = "http://localhost:1234/v1"
)

// LMStudio is a Genkit plugin that connects to LM Studio's local API.
//
// Pass it to genkit.Init via genkit.WithPlugins:
//
//	g := genkit.Init(ctx, genkit.WithPlugins(&lmstudio.LMStudio{
//	    Models: []lmstudio.ModelDef{{Name: "google/gemma-3-4b"}},
//	}))
type LMStudio struct {
	// BaseURL is the LM Studio server URL. Defaults to http://localhost:1234/v1.
	BaseURL string

	// Models lists the models to register. Each model must be loaded in LM Studio.
	Models []ModelDef

	compat *compat_oai.OpenAICompatible
}

// ModelDef defines a model available in LM Studio.
type ModelDef struct {
	// Name is the model identifier as shown in LM Studio (e.g. "google/gemma-3-4b").
	// The Genkit model reference will be "lmstudio/<Name>".
	Name string

	// Supports declares what the model supports (tools, structured output, etc.).
	// Defaults to basic text generation if nil.
	Supports *ai.ModelSupports
}

func (l *LMStudio) Name() string { return provider }

func (l *LMStudio) Init(ctx context.Context) []api.Action {
	baseURL := l.BaseURL
	if baseURL == "" {
		baseURL = DefaultURL
	}
	l.compat = &compat_oai.OpenAICompatible{
		Provider: provider,
		APIKey:   "lm-studio",
		BaseURL:  baseURL,
	}
	actions := l.compat.Init(ctx)
	for _, m := range l.Models {
		supports := m.Supports
		if supports == nil {
			supports = &compat_oai.BasicText
		}
		model := l.compat.DefineModel(provider, m.Name, ai.ModelOptions{
			Label:    "LM Studio: " + m.Name,
			Supports: supports,
		})
		actions = append(actions, model.(api.Action))
	}
	return actions
}
