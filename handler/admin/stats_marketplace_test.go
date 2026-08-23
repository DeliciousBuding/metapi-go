package admin

import (
	"reflect"
	"testing"

	"github.com/deliciousbuding/metapi-go/service/pricingcatalog"
)

// inferCatalogSnapshot builds a catalog snapshot from llm-metadata-shaped
// JSON (the exported parser is the only way handler tests can construct one).
func inferCatalogSnapshot(t *testing.T, payload string) *pricingcatalog.CatalogSnapshot {
	t.Helper()
	snapshot, err := pricingcatalog.ParseLLMMetadata([]byte(payload))
	if err != nil {
		t.Fatalf("ParseLLMMetadata: %v", err)
	}
	return snapshot
}

// TestInferEndpointTypes_FromCatalogData replaces the name-prefix guess with
// catalog provider (dialect) + directed modalities (endpoint families).
func TestInferEndpointTypes_FromCatalogData(t *testing.T) {
	snapshot := inferCatalogSnapshot(t, `{
	  "openai": {"id": "openai", "models": {
	    "gpt-4o": {"id": "gpt-4o", "modalities": {"input": ["text", "image", "pdf"], "output": ["text"]}},
	    "gpt-image-1": {"id": "gpt-image-1", "modalities": {"input": ["text", "image"], "output": ["image"]}},
	    "text-embedding-3-large": {"id": "text-embedding-3-large", "modalities": {"input": ["text"], "output": ["text"]}}
	  }},
	  "anthropic": {"id": "anthropic", "models": {
	    "claude-sonnet-4-20250514": {"id": "claude-sonnet-4-20250514", "modalities": {"input": ["text", "image"], "output": ["text"]}}
	  }},
	  "google": {"id": "google", "models": {
	    "gemini-2.5-pro": {"id": "gemini-2.5-pro", "modalities": {"input": ["text", "image", "audio", "video", "pdf"], "output": ["text"]}},
	    "gemini-3.1-flash-tts-preview": {"id": "gemini-3.1-flash-tts-preview", "modalities": {"input": ["text"], "output": ["audio"]}},
	    "gemini-embedding-2": {"id": "gemini-embedding-2", "modalities": {"input": ["text"], "output": ["text"]}}
	  }},
	  "deepseek": {"id": "deepseek", "models": {
	    "deepseek-chat": {"id": "deepseek-chat", "modalities": {"input": ["text"], "output": ["text"]}}
	  }}
	}`)

	cases := map[string][]string{
		// Vision chat: image in, text out → chat surface only, dialect from
		// catalog provider (not the name).
		"gpt-4o": {"openai"},
		// Image generator: image output → generation + edit surfaces.
		"gpt-image-1": {"openai", "images.generations", "images.edits"},
		// Embedding: no modality signal → family from name.
		"text-embedding-3-large": {"openai", "embeddings"},
		// Claude dialect from catalog provider.
		"claude-sonnet-4-20250514": {"anthropic"},
		// Audio input (transcription) + video input: audio family surfaces.
		"gemini-2.5-pro": {"gemini", "audio.transcriptions"},
		// TTS: audio output → speech family.
		"gemini-3.1-flash-tts-preview": {"gemini", "audio.speech"},
		// Embedding model under google: dialect + name family.
		"gemini-embedding-2": {"gemini", "embeddings"},
		// Non-big-three provider: OpenAI-compatible dialect default replaces
		// the old empty heuristic.
		"deepseek-chat": {"openai"},
	}

	for model, want := range cases {
		got := inferEndpointTypesForModel(snapshot, model, nil)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("inferEndpointTypesForModel(%q) = %v, want %v", model, got, want)
		}
	}

	// Catalog miss keeps the name-prefix heuristic as fallback.
	got := inferEndpointTypesForModel(snapshot, "claude-nickname-xyz", nil)
	if !reflect.DeepEqual(got, []string{"anthropic"}) {
		t.Errorf("catalog-miss claude = %v, want [anthropic]", got)
	}
	got = inferEndpointTypesForModel(snapshot, "totally-unknown-model", nil)
	if len(got) != 0 {
		t.Errorf("catalog-miss unknown = %v, want empty", got)
	}
}

// TestInferEndpointTypes_RerankAndNilSnapshot covers the rerank family and
// the nil-snapshot path (catalog disabled in tests).
func TestInferEndpointTypes_RerankAndNilSnapshot(t *testing.T) {
	rerank := inferCatalogSnapshot(t, `{
	  "cohere": {"id": "cohere", "models": {
	    "rerank-english-v3": {"id": "rerank-english-v3", "modalities": {"input": ["text"], "output": ["text"]}}
	  }}
	}`)
	got := inferEndpointTypesForModel(rerank, "rerank-english-v3", nil)
	if !reflect.DeepEqual(got, []string{"openai", "rerank"}) {
		t.Errorf("rerank = %v, want [openai rerank]", got)
	}

	// nil snapshot (catalog disabled) → heuristics only.
	got = inferEndpointTypesForModel(nil, "gpt-4o", nil)
	if !reflect.DeepEqual(got, []string{"openai"}) {
		t.Errorf("nil snapshot gpt-4o = %v, want [openai]", got)
	}
	got = inferEndpointTypesForModel(nil, "gemini-2.5-pro", nil)
	if !reflect.DeepEqual(got, []string{"gemini"}) {
		t.Errorf("nil snapshot gemini = %v, want [gemini]", got)
	}
}
